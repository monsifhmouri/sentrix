// Package cli implements the non-interactive command line. It uses the
// standard flag package: Cobra was the preferred choice but is not
// available in the offline build environment (see docs/dependencies.md).
package cli

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"sentrix/internal/app"
	"sentrix/internal/buildinfo"
	"sentrix/internal/config"
	"sentrix/internal/storage"
	"sentrix/internal/tui"
	"sentrix/pkg/models"
)

const usage = `Usage: sentrix [global flags] [command]

Run with no command to open the interactive menu.

Commands:
  scan       Create a project and run a passive scan
  rescan     Re-run a scan for an existing project
  projects   List saved projects
  show       Browse and filter a project's records (offline)
  export     Export a project (json, csv, txt, html)
  sources    List data sources (add -check for a live availability test)
  check      Verify a project's integrity
  backup     Copy a project to a backup file
  restore    Restore a project from a backup file
  config     Show the configuration (secrets masked)
  version    Print version information
  about      About SENTRIX

Global flags:
  -config PATH   configuration file (default ~/.sentrix/config.json)
  -no-anim       disable the startup animation
  -no-color      disable colour output

Run "sentrix <command> -h" for command flags.
`

type listFlag []string

func (l *listFlag) String() string     { return strings.Join(*l, ",") }
func (l *listFlag) Set(v string) error { *l = append(*l, tui.SplitList(v)...); return nil }

// Run executes the CLI and returns the process exit code.
func Run(args []string, in io.Reader, stdout, stderr io.Writer) int {
	global := flag.NewFlagSet("sentrix", flag.ContinueOnError)
	global.SetOutput(stderr)
	global.Usage = func() { fmt.Fprint(stderr, usage) }
	cfgPath := global.String("config", config.DefaultPath(), "")
	noAnim := global.Bool("no-anim", false, "")
	noColor := global.Bool("no-color", false, "")
	if err := global.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}
	rest := global.Args()

	a, err := app.New(*cfgPath)
	if err != nil {
		fmt.Fprintln(stderr, "error:", err)
		return 1
	}
	theme := tui.Theme{}
	if f, ok := stdout.(*os.File); ok {
		theme = tui.DetectTheme(a.Cfg.Theme, f)
	}
	if *noColor {
		theme.Color = false
	}
	animate := a.Cfg.Animation && !*noAnim && theme.Color

	c := &cmd{a: a, in: in, out: stdout, err: stderr, t: theme}
	if len(rest) == 0 {
		tui.Banner(stdout, theme, animate)
		m := &tui.Menu{App: a, P: tui.NewPrompter(in, stdout, theme), T: theme}
		if err := m.Run(); err != nil {
			fmt.Fprintln(stderr, "error:", err)
			return 1
		}
		return 0
	}

	name, cargs := rest[0], rest[1:]
	handlers := map[string]func([]string) error{
		"scan": c.scan, "rescan": c.rescan, "projects": c.projects, "show": c.show, "export": c.export,
		"sources": c.sources, "check": c.check, "backup": c.backup, "restore": c.restore, "config": c.config,
		"version": func([]string) error { fmt.Fprintln(stdout, buildinfo.Summary()); return nil },
		"about":   func([]string) error { tui.Banner(stdout, theme, false); tui.About(stdout, theme); return nil },
	}
	if name == "help" {
		fmt.Fprint(stdout, usage)
		return 0
	}
	h, ok := handlers[name]
	if !ok {
		fmt.Fprintf(stderr, "unknown command %q\n\n%s", name, usage)
		return 2
	}
	if err := h(cargs); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		fmt.Fprintln(stderr, "error:", err)
		return 1
	}
	return 0
}

type cmd struct {
	a        *app.App
	in       io.Reader
	out, err io.Writer
	t        tui.Theme
}

func (c *cmd) flags(name string) *flag.FlagSet {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(c.err)
	return fs
}

func (c *cmd) load(name string) (*models.Project, error) {
	if name == "" {
		return nil, errors.New("-name is required")
	}
	return c.a.Store.Load(name)
}

func (c *cmd) scan(args []string) error {
	fs := c.flags("scan")
	var in app.ProjectInput
	var scopeEntries, srcs, formats listFlag
	fs.StringVar(&in.Name, "name", "", "project name (required)")
	fs.StringVar(&in.Domain, "domain", "", "target domain (required)")
	fs.Var(&scopeEntries, "scope", "extra scope entry, repeatable: host:X | url:https://h/p | root:D | exclude:X")
	fs.StringVar(&in.Program, "program", "", "program or organization name")
	fs.StringVar(&in.Mode, "mode", "", "passive | passive-extended")
	fs.Var(&srcs, "sources", "comma-separated sources (default: all enabled)")
	fs.IntVar(&in.Concurrency, "concurrency", 0, "max concurrent sources (1-32)")
	fs.Float64Var(&in.RatePerSecond, "rate", 0, "max requests per second per source")
	fs.StringVar(&in.OutputDir, "out", "", "output directory")
	fs.Var(&formats, "format", "export formats: json,csv,txt,html")
	fs.StringVar(&in.DedupeMode, "dedupe", "", "strict | params (default params)")
	yes := fs.Bool("yes", false, "skip the confirmation prompt (you still assert authorization)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	in.ScopeEntries, in.Sources, in.Formats = scopeEntries, srcs, formats
	p, err := c.a.NewProject(in)
	if err != nil {
		return err
	}
	return c.confirmAndScan(p, *yes)
}

func (c *cmd) confirmAndScan(p *models.Project, yes bool) error {
	fmt.Fprint(c.out, c.t.Header("Review"))
	tui.PrintReview(c.out, c.t, p)
	if !yes {
		ok, err := tui.NewPrompter(c.in, c.out, c.t).Confirm("\nI am authorized to test this scope. Start the passive scan?", false)
		if err != nil || !ok {
			fmt.Fprintln(c.out, "cancelled; nothing was saved or sent.")
			return err
		}
	}
	ctx, stop := tui.WithInterrupt()
	defer stop()
	fmt.Fprint(c.out, c.t.Header("Scanning (Ctrl-C stops and keeps partial results)"))
	sum, err := c.a.Scan(ctx, p, tui.NewProgress(c.out, c.t).Notify)
	tui.PrintSummary(c.out, c.t, sum)
	if err != nil && !errors.Is(err, context.Canceled) {
		return err
	}
	if errors.Is(err, context.Canceled) {
		fmt.Fprintln(c.out, "scan interrupted; partial results were saved")
	}
	paths, err := c.a.Export(p, nil, "")
	for _, path := range paths {
		fmt.Fprintln(c.out, "exported", path)
	}
	return err
}

func (c *cmd) rescan(args []string) error {
	fs := c.flags("rescan")
	name := fs.String("name", "", "project name (required)")
	yes := fs.Bool("yes", false, "skip the confirmation prompt")
	if err := fs.Parse(args); err != nil {
		return err
	}
	p, err := c.load(*name)
	if err != nil {
		return err
	}
	return c.confirmAndScan(p, *yes)
}

func (c *cmd) projects(args []string) error {
	list, err := c.a.Store.List()
	if err != nil {
		return err
	}
	if len(list) == 0 {
		fmt.Fprintln(c.out, "no projects")
	}
	for _, s := range list {
		fmt.Fprintf(c.out, "%-24s %-28s %6d records  updated %s\n", s.Name, s.Domain, s.Records, s.UpdatedAt.Format("2006-01-02 15:04"))
	}
	return nil
}

func (c *cmd) show(args []string) error {
	fs := c.flags("show")
	name := fs.String("name", "", "project name (required)")
	text := fs.String("q", "", "search text")
	class := fs.String("class", "", "class filter")
	source := fs.String("source", "", "source filter")
	scopeF := fs.String("scope", "", "in | out")
	kind := fs.String("kind", "", "host | endpoint")
	limit := fs.Int("limit", 100, "max rows")
	if err := fs.Parse(args); err != nil {
		return err
	}
	p, err := c.load(*name)
	if err != nil {
		return err
	}
	q := storage.Query{Text: *text, Class: *class, Source: *source, Kind: models.Kind(*kind)}
	switch *scopeF {
	case "in":
		q.Scope = models.InScope
	case "out":
		q.Scope = models.OutOfScope
	case "":
	default:
		return errors.New("-scope must be in or out")
	}
	res := storage.Filter(p.Records, q)
	fmt.Fprintf(c.out, "%d of %d records\n", len(res), len(p.Records))
	tui.PrintRecords(c.out, c.t, res, *limit)
	return nil
}

func (c *cmd) export(args []string) error {
	fs := c.flags("export")
	name := fs.String("name", "", "project name (required)")
	out := fs.String("out", "", "output directory (default: project output dir)")
	var formats listFlag
	fs.Var(&formats, "format", "formats: json,csv,txt,html (default: project setting)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	p, err := c.load(*name)
	if err != nil {
		return err
	}
	if err := config.ValidFormats(formats); err != nil {
		return err
	}
	paths, err := c.a.Export(p, formats, *out)
	for _, path := range paths {
		fmt.Fprintln(c.out, "exported", path)
	}
	return err
}

func (c *cmd) sources(args []string) error {
	fs := c.flags("sources")
	check := fs.Bool("check", false, "run a live availability check (sends one request per source)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	for _, s := range c.a.SourceStatuses() {
		key := "-"
		if s.Access == "optional-key" {
			key = map[bool]string{true: "key set", false: "no key"}[s.KeyConfigured]
		}
		fmt.Fprintf(c.out, "%-12s %-13s enabled=%-5t %-7s %s\n", s.Name, s.Access, s.Enabled, key, s.Description)
	}
	if !*check {
		return nil
	}
	ctx, stop := tui.WithInterrupt()
	defer stop()
	res := c.a.CheckSources(ctx)
	for _, s := range c.a.SourceStatuses() {
		if err := res[s.Name]; err != nil {
			fmt.Fprintf(c.out, "%s %-12s %v\n", c.t.Mark("fail"), s.Name, err)
		} else {
			fmt.Fprintf(c.out, "%s %-12s reachable\n", c.t.Mark("ok"), s.Name)
		}
	}
	return nil
}

func (c *cmd) check(args []string) error {
	fs := c.flags("check")
	name := fs.String("name", "", "project name (required)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *name == "" {
		return errors.New("-name is required")
	}
	if err := c.a.Store.Check(*name); err != nil {
		return err
	}
	fmt.Fprintln(c.out, "integrity OK")
	return nil
}

func (c *cmd) backup(args []string) error {
	fs := c.flags("backup")
	name := fs.String("name", "", "project name (required)")
	to := fs.String("to", "", "backup file path (required)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *name == "" || *to == "" {
		return errors.New("-name and -to are required")
	}
	if err := c.a.Store.Backup(*name, *to); err != nil {
		return err
	}
	fmt.Fprintln(c.out, "backup written to", *to)
	return nil
}

func (c *cmd) restore(args []string) error {
	fs := c.flags("restore")
	from := fs.String("from", "", "backup file path (required)")
	overwrite := fs.Bool("overwrite", false, "replace an existing project")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *from == "" {
		return errors.New("-from is required")
	}
	name, err := c.a.Store.Restore(*from, *overwrite)
	if err != nil {
		return err
	}
	fmt.Fprintln(c.out, "restored project", name)
	return nil
}

func (c *cmd) config(args []string) error {
	fmt.Fprintln(c.out, "config file:", c.a.CfgPath)
	r := c.a.Cfg.Redacted()
	b, err := jsonIndent(r)
	if err != nil {
		return err
	}
	fmt.Fprintln(c.out, b)
	return nil
}
