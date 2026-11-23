package tui

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strconv"
	"strings"

	"sentrix/internal/app"
	"sentrix/internal/buildinfo"
	"sentrix/internal/config"
	"sentrix/internal/scope"
	"sentrix/internal/storage"
	"sentrix/pkg/models"
)

// Menu is the interactive main menu.
type Menu struct {
	App *app.App
	P   *Prompter
	T   Theme
}

// Run shows the main menu until the user exits or input ends.
func (m *Menu) Run() error {
	for {
		w := m.P.Out
		fmt.Fprint(w, m.T.Header("Main menu"))
		fmt.Fprintln(w, "  [1] Create New Reconnaissance Project\n  [2] Resume Existing Project\n  [3] Browse Previous Results")
		fmt.Fprintln(w, "  [4] Manage Data Sources\n  [5] Configuration\n  [6] Update Information\n  [7] About SENTRIX\n  [0] Exit")
		choice, err := m.P.Ask("\nSelect:", "")
		if err != nil {
			return ignoreEOF(err)
		}
		switch choice {
		case "1":
			err = m.createProject()
		case "2":
			err = m.resume()
		case "3":
			err = m.browse()
		case "4":
			err = m.sources()
		case "5":
			err = m.configuration()
		case "6":
			m.update()
		case "7":
			About(w, m.T)
		case "0", "q", "exit":
			return nil
		default:
			fmt.Fprintln(w, m.T.Err("  unknown option"))
		}
		if err != nil {
			if errors.Is(err, io.EOF) {
				return nil
			}
			fmt.Fprintln(w, "  "+m.T.Err("error: ")+err.Error())
		}
	}
}

func ignoreEOF(err error) error {
	if errors.Is(err, io.EOF) {
		return nil
	}
	return err
}

func (m *Menu) createProject() error {
	a, p, w := m.App, m.P, m.P.Out
	fmt.Fprint(w, m.T.Header("New reconnaissance project"))
	fmt.Fprintln(w, m.T.Dim("  Nothing is sent over the network until you confirm at the end.\n"))

	var in app.ProjectInput
	var err error
	if in.Name, err = p.AskValid("1. Project name:", "", a.ValidateName); err != nil {
		return err
	}
	if in.Domain, err = p.AskValid("2. Target domain:", "", func(s string) error {
		_, err := scope.ParseRules(s, nil)
		return err
	}); err != nil {
		return err
	}
	fmt.Fprintln(w, m.T.Dim("   Scope entries: host:NAME  url:https://host/prefix  root:DOMAIN  exclude:HOST-or-URL"))
	var entries string
	if _, err = p.AskValid("3. Additional scope (comma separated, blank for none):", "", func(s string) error {
		entries = s
		_, err := scope.ParseRules(in.Domain, SplitList(s))
		return err
	}); err != nil {
		return err
	}
	in.ScopeEntries = SplitList(entries)
	if in.Program, err = p.Ask("4. Program or organization (optional):", ""); err != nil {
		return err
	}
	if in.Mode, err = p.AskValid("5. Discovery mode (passive / passive-extended):", app.ModePassive, func(s string) error {
		if s != app.ModePassive && s != app.ModePassiveExtended {
			return errors.New("choose passive or passive-extended")
		}
		return nil
	}); err != nil {
		return err
	}
	var src string
	if src, err = p.AskValid("6. Data sources (comma separated):", strings.Join(a.EnabledSources(), ","), func(s string) error {
		if len(SplitList(s)) == 0 {
			return errors.New("select at least one source")
		}
		return nil
	}); err != nil {
		return err
	}
	in.Sources = SplitList(src)
	var conc string
	if conc, err = p.AskValid("7. Maximum concurrency (1-32):", strconv.Itoa(a.Cfg.Concurrency), func(s string) error {
		n, e := strconv.Atoi(s)
		if e != nil || n < 1 || n > 32 {
			return errors.New("enter a number from 1 to 32")
		}
		return nil
	}); err != nil {
		return err
	}
	in.Concurrency, _ = strconv.Atoi(conc)
	var rate string
	if rate, err = p.AskValid("8. Max requests per second per source:", strconv.FormatFloat(a.Cfg.RatePerSecond, 'f', -1, 64), func(s string) error {
		f, e := strconv.ParseFloat(s, 64)
		if e != nil || f < 0.05 || f > 50 {
			return errors.New("enter a number from 0.05 to 50")
		}
		return nil
	}); err != nil {
		return err
	}
	in.RatePerSecond, _ = strconv.ParseFloat(rate, 64)
	if in.OutputDir, err = p.AskValid("9. Output directory:", a.Cfg.OutputDir, func(s string) error {
		if strings.TrimSpace(s) == "" {
			return errors.New("required")
		}
		return nil
	}); err != nil {
		return err
	}
	var formats string
	if formats, err = p.AskValid("10. Export formats (json,csv,txt,html):", strings.Join(a.Cfg.Formats, ","), func(s string) error {
		return config.ValidFormats(SplitList(s))
	}); err != nil {
		return err
	}
	in.Formats = SplitList(formats)

	proj, err := a.NewProject(in)
	if err != nil {
		return err
	}
	fmt.Fprint(w, m.T.Header("11. Review"))
	PrintReview(w, m.T, proj)
	ok, err := p.Confirm("\nI am authorized to test this scope. Start the passive scan?", false)
	if err != nil || !ok {
		fmt.Fprintln(w, "  cancelled; nothing was saved or sent.")
		return err
	}
	return m.scanAndReport(proj)
}

// PrintReview shows the configuration a user must approve before any network use.
func PrintReview(w io.Writer, t Theme, p *models.Project) {
	fmt.Fprintf(w, "  Project:      %s\n  Target:       %s\n  Mode:         %s\n", p.Name, p.Domain, p.Mode)
	if p.Program != "" {
		fmt.Fprintf(w, "  Program:      %s\n", p.Program)
	}
	fmt.Fprintf(w, "  In scope:     roots %s", strings.Join(p.Scope.Roots, ", "))
	if len(p.Scope.Hosts) > 0 {
		fmt.Fprintf(w, "; hosts %s", strings.Join(p.Scope.Hosts, ", "))
	}
	if len(p.Scope.URLPrefixes) > 0 {
		fmt.Fprintf(w, "; URL prefixes %s", strings.Join(p.Scope.URLPrefixes, ", "))
	}
	fmt.Fprintln(w)
	if len(p.Scope.Exclude) > 0 {
		fmt.Fprintf(w, "  Excluded:     %s\n", strings.Join(p.Scope.Exclude, ", "))
	}
	fmt.Fprintf(w, "  Sources:      %s\n  Concurrency:  %d   Rate: %g req/s per source\n  Output:       %s (%s)\n",
		strings.Join(p.Sources, ", "), p.Concurrency, p.RatePerSecond, p.OutputDir, strings.Join(p.ExportFormats, ", "))
	fmt.Fprintln(w, t.Dim("  Requests go to the third-party datasets above only, never to the target itself."))
}

// WithInterrupt returns a context cancelled by the first Ctrl-C.
func WithInterrupt() (context.Context, context.CancelFunc) {
	return signal.NotifyContext(context.Background(), os.Interrupt)
}

func (m *Menu) scanAndReport(p *models.Project) error {
	w := m.P.Out
	ctx, stop := WithInterrupt()
	defer stop()
	fmt.Fprint(w, m.T.Header("Scanning (Ctrl-C stops and keeps partial results)"))
	sum, err := m.App.Scan(ctx, p, NewProgress(w, m.T).Notify)
	PrintSummary(w, m.T, sum)
	if err != nil && !errors.Is(err, context.Canceled) {
		return err
	}
	if errors.Is(err, context.Canceled) {
		fmt.Fprintln(w, "  "+m.T.Warn("scan interrupted; partial results were saved"))
	}
	paths, xerr := m.App.Export(p, nil, "")
	for _, path := range paths {
		fmt.Fprintf(w, "  exported %s\n", path)
	}
	return xerr
}

func (m *Menu) pickProject(label string) (*models.Project, error) {
	list, err := m.App.Store.List()
	if err != nil {
		return nil, err
	}
	w := m.P.Out
	if len(list) == 0 {
		fmt.Fprintln(w, "  no projects yet - create one first")
		return nil, nil
	}
	fmt.Fprint(w, m.T.Header(label))
	for i, s := range list {
		fmt.Fprintf(w, "  [%d] %s  %s\n", i+1, s.Name, m.T.Dim(fmt.Sprintf("%s, %d records", s.Domain, s.Records)))
	}
	ans, err := m.P.Ask("\nProject number (0 to go back):", "")
	if err != nil {
		return nil, err
	}
	n, e := strconv.Atoi(ans)
	if e != nil || n < 1 || n > len(list) {
		return nil, nil
	}
	return m.App.Store.Load(list[n-1].Name)
}

func (m *Menu) resume() error {
	p, err := m.pickProject("Resume project")
	if p == nil || err != nil {
		return err
	}
	w := m.P.Out
	PrintReview(w, m.T, p)
	fmt.Fprintf(w, "  Snapshots:    %d previous scan(s)\n", len(p.Snapshots))
	ok, err := m.P.Confirm("\nRe-run the passive scan with this configuration?", false)
	if err != nil || !ok {
		return err
	}
	return m.scanAndReport(p)
}

func (m *Menu) browse() error {
	p, err := m.pickProject("Browse results")
	if p == nil || err != nil {
		return err
	}
	w := m.P.Out
	fmt.Fprintf(w, "  %d records (works offline - no network is used)\n", len(p.Records))
	for {
		text, err := m.P.Ask("Search text (blank = all, 0 = back):", "")
		if err != nil || text == "0" {
			return err
		}
		class, err := m.P.Ask("Class filter (e.g. graphql, rest-api, javascript; blank = any):", "")
		if err != nil {
			return err
		}
		sc, err := m.P.Ask("Scope filter (in / out / blank = any):", "")
		if err != nil {
			return err
		}
		q := storage.Query{Text: text, Class: class}
		switch sc {
		case "in":
			q.Scope = models.InScope
		case "out":
			q.Scope = models.OutOfScope
		}
		res := storage.Filter(p.Records, q)
		fmt.Fprintf(w, "\n  %d match(es)\n", len(res))
		PrintRecords(w, m.T, res, 40)
	}
}

func (m *Menu) sources() error {
	w := m.P.Out
	fmt.Fprint(w, m.T.Header("Data sources"))
	fmt.Fprintf(w, "  %-12s %-13s %-8s %-8s %s\n", "SOURCE", "ACCESS", "ENABLED", "KEY", "DESCRIPTION")
	for _, s := range m.App.SourceStatuses() {
		key := "n/a"
		if s.Access == "optional-key" {
			key = map[bool]string{true: "set", false: "not set"}[s.KeyConfigured]
		}
		fmt.Fprintf(w, "  %-12s %-13s %-8t %-8s %s\n", s.Name, s.Access, s.Enabled, key, s.Description)
	}
	fmt.Fprintln(w, m.T.Dim("\n  Availability is not assumed. Press c to run a live check (sends one small request per source)."))
	ans, err := m.P.Ask("[c] live check   [Enter] back:", "")
	if err != nil || strings.ToLower(ans) != "c" {
		return err
	}
	ctx, stop := WithInterrupt()
	defer stop()
	for name, cerr := range m.App.CheckSources(ctx) {
		if cerr != nil {
			fmt.Fprintf(w, "  %s %-12s %v\n", m.T.Mark("fail"), name, cerr)
		} else {
			fmt.Fprintf(w, "  %s %-12s reachable\n", m.T.Mark("ok"), name)
		}
	}
	return nil
}

func (m *Menu) configuration() error {
	a, w := m.App, m.P.Out
	for {
		c := a.Cfg
		fmt.Fprint(w, m.T.Header("Configuration"))
		fmt.Fprintf(w, "  file:         %s\n  data dir:     %s\n  output dir:   %s\n  concurrency:  %d   rate: %g req/s\n  formats:      %s\n  log level:    %s   theme: %s   animation: %t\n",
			a.CfgPath, c.DataDir, c.OutputDir, c.Concurrency, c.RatePerSecond, strings.Join(c.Formats, ","), c.LogLevel, c.Theme, c.Animation)
		for _, s := range a.SourceStatuses() {
			fmt.Fprintf(w, "  source %-12s enabled=%t key=%s\n", s.Name, s.Enabled, map[bool]string{true: "set", false: "not set"}[s.KeyConfigured])
		}
		fmt.Fprintln(w, "\n  [1] Toggle a source   [2] Set an API key   [3] Default concurrency   [0] Back")
		ans, err := m.P.Ask("Select:", "")
		if err != nil || ans == "0" || ans == "" {
			return err
		}
		switch ans {
		case "1", "2":
			name, err := m.P.Ask("Source name:", "")
			if err != nil {
				return err
			}
			sc, ok := a.Cfg.Sources[name]
			if !ok {
				fmt.Fprintln(w, m.T.Err("  unknown source"))
				continue
			}
			if ans == "1" {
				sc.Enabled = !sc.Enabled
			} else {
				key, err := m.P.Ask("API key (stored in the config file; prefer the SENTRIX_"+strings.ToUpper(name)+"_API_KEY variable):", "")
				if err != nil {
					return err
				}
				sc.APIKey = key
			}
			a.Cfg.Sources[name] = sc
		case "3":
			v, err := m.P.AskValid("Concurrency (1-32):", strconv.Itoa(c.Concurrency), func(s string) error {
				n, e := strconv.Atoi(s)
				if e != nil || n < 1 || n > 32 {
					return errors.New("enter a number from 1 to 32")
				}
				return nil
			})
			if err != nil {
				return err
			}
			a.Cfg.Concurrency, _ = strconv.Atoi(v)
		default:
			continue
		}
		if err := a.Cfg.Save(a.CfgPath); err != nil {
			return err
		}
	}
}

func (m *Menu) update() {
	w := m.P.Out
	fmt.Fprint(w, m.T.Header("Update information"))
	fmt.Fprintf(w, "  Installed: %s\n", buildinfo.Summary())
	fmt.Fprintln(w, "  SENTRIX never checks for updates or phones home. To update, rebuild from source")
	fmt.Fprintf(w, "  or download a new release published by the developer: %s\n", buildinfo.GitHub)
}
