// Package app wires configuration, storage, sources and the discovery
// pipeline into the operations the CLI and the interactive UI both use.
package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"sentrix/internal/config"
	"sentrix/internal/deduplication"
	"sentrix/internal/discovery"
	"sentrix/internal/logging"
	"sentrix/internal/reporting"
	"sentrix/internal/scope"
	"sentrix/internal/sources"
	"sentrix/internal/storage"
	"sentrix/pkg/models"
)

const (
	ModePassive         = "passive"
	ModePassiveExtended = "passive-extended"
)

var maxResults = map[string]int{ModePassive: 5000, ModePassiveExtended: 50000}

type App struct {
	Cfg     config.Config
	CfgPath string
	Store   *storage.Store
	Log     *slog.Logger

	baseURLs map[string]string // test hook: redirects a source to a local mock
}

// New loads the configuration at cfgPath (defaults if absent) and opens storage.
func New(cfgPath string) (*App, error) {
	cfg, err := config.Load(cfgPath)
	if err != nil {
		return nil, err
	}
	store, err := storage.Open(cfg.DataDir)
	if err != nil {
		return nil, err
	}
	return &App{Cfg: cfg, CfgPath: cfgPath, Store: store, Log: logging.New(os.Stderr, cfg.LogLevel)}, nil
}

// ProjectInput is everything the user supplies when creating a project.
type ProjectInput struct {
	Name, Domain, Program, Mode string
	ScopeEntries                []string
	Sources                     []string
	Concurrency                 int
	RatePerSecond               float64
	OutputDir                   string
	Formats                     []string
	DedupeMode                  string
}

// Defaults fills unset fields from the configuration.
func (a *App) Defaults(in ProjectInput) ProjectInput {
	if in.Mode == "" {
		in.Mode = ModePassive
	}
	if len(in.Sources) == 0 {
		in.Sources = a.EnabledSources()
	}
	if in.Concurrency == 0 {
		in.Concurrency = a.Cfg.Concurrency
	}
	if in.RatePerSecond == 0 {
		in.RatePerSecond = a.Cfg.RatePerSecond
	}
	if in.OutputDir == "" {
		in.OutputDir = a.Cfg.OutputDir
	}
	if len(in.Formats) == 0 {
		in.Formats = a.Cfg.Formats
	}
	if in.DedupeMode == "" {
		in.DedupeMode = string(deduplication.ParamNames)
	}
	return in
}

// ValidateName checks a project name and that it is not already taken.
func (a *App) ValidateName(name string) error {
	if _, err := storage.Slug(name); err != nil {
		return err
	}
	if a.Store.Exists(name) {
		return fmt.Errorf("a project named %q already exists", name)
	}
	return nil
}

// NewProject validates the input and returns an unsaved project.
func (a *App) NewProject(in ProjectInput) (*models.Project, error) {
	in = a.Defaults(in)
	if err := a.ValidateName(in.Name); err != nil {
		return nil, err
	}
	rules, err := scope.ParseRules(in.Domain, in.ScopeEntries)
	if err != nil {
		return nil, err
	}
	if _, ok := maxResults[in.Mode]; !ok {
		return nil, fmt.Errorf("unknown mode %q (use %s or %s)", in.Mode, ModePassive, ModePassiveExtended)
	}
	if _, ok := deduplication.ParseMode(in.DedupeMode); !ok {
		return nil, fmt.Errorf("unknown dedupe mode %q (use strict or params)", in.DedupeMode)
	}
	for _, s := range in.Sources {
		if _, err := sources.Build(s, sources.Settings{}); err != nil {
			return nil, err
		}
	}
	if in.Concurrency < 1 || in.Concurrency > 32 {
		return nil, errors.New("concurrency must be between 1 and 32")
	}
	if in.RatePerSecond < 0.05 || in.RatePerSecond > 50 {
		return nil, errors.New("rate limit must be between 0.05 and 50 requests per second per source")
	}
	if err := config.ValidFormats(in.Formats); err != nil {
		return nil, err
	}
	if strings.TrimSpace(in.OutputDir) == "" {
		return nil, errors.New("output directory is required")
	}
	now := time.Now().UTC()
	return &models.Project{
		Name: strings.TrimSpace(in.Name), Domain: rules.Roots[0], Program: strings.TrimSpace(in.Program), Mode: in.Mode,
		Scope: rules, Sources: in.Sources, Concurrency: in.Concurrency, RatePerSecond: in.RatePerSecond,
		DedupeMode: in.DedupeMode, OutputDir: config.ExpandHome(in.OutputDir), ExportFormats: in.Formats,
		CreatedAt: now, UpdatedAt: now,
	}, nil
}

// EnabledSources lists source names enabled in the configuration.
func (a *App) EnabledSources() []string {
	var out []string
	for _, info := range sources.Catalog() {
		if a.Cfg.Sources[info.Name].Enabled {
			out = append(out, info.Name)
		}
	}
	return out
}

// SourceStatus is offline information about one source.
type SourceStatus struct {
	sources.Info
	Enabled       bool
	KeyConfigured bool
}

func (a *App) SourceStatuses() []SourceStatus {
	var out []SourceStatus
	for _, info := range sources.Catalog() {
		out = append(out, SourceStatus{Info: info, Enabled: a.Cfg.Sources[info.Name].Enabled, KeyConfigured: a.Cfg.APIKey(info.Name) != ""})
	}
	return out
}

func (a *App) httpClient() (*http.Client, error) {
	tr := http.DefaultTransport.(*http.Transport).Clone()
	tr.Proxy = nil // proxies are used only when explicitly configured
	if a.Cfg.Proxy != "" {
		u, err := url.Parse(a.Cfg.Proxy)
		if err != nil {
			return nil, err
		}
		tr.Proxy = http.ProxyURL(u)
	}
	return &http.Client{Transport: tr}, nil
}

func (a *App) buildSources(p *models.Project) ([]sources.Source, time.Duration, error) {
	client, err := a.httpClient()
	if err != nil {
		return nil, 0, err
	}
	var out []sources.Source
	var timeout time.Duration
	for _, name := range p.Sources {
		sc := a.Cfg.Sources[name]
		rate := sc.RatePerSecond
		if p.RatePerSecond > 0 && (rate == 0 || p.RatePerSecond < rate) {
			rate = p.RatePerSecond
		}
		s, err := sources.Build(name, sources.Settings{
			BaseURL: a.baseURLs[name], APIKey: a.Cfg.APIKey(name), Client: client, RatePerSecond: rate, MaxResults: maxResults[p.Mode],
		})
		if err != nil {
			return nil, 0, err
		}
		out = append(out, s)
		timeout = max(timeout, time.Duration(sc.TimeoutSeconds)*time.Second)
	}
	return out, timeout, nil
}

// Scan runs the project's sources and saves the result. The project is saved
// even when the scan is interrupted, so partial work is never lost.
func (a *App) Scan(ctx context.Context, p *models.Project, notify func(discovery.Event)) (discovery.Summary, error) {
	srcs, timeout, err := a.buildSources(p)
	if err != nil {
		return discovery.Summary{}, err
	}
	mode, _ := deduplication.ParseMode(p.DedupeMode)
	sum, runErr := discovery.Run(ctx, p, srcs, discovery.Options{
		Concurrency: p.Concurrency, Timeout: timeout, Dedupe: mode, Notify: notify,
	})
	if err := a.Store.Save(p); err != nil {
		return sum, errors.Join(runErr, err)
	}
	return sum, runErr
}

// CheckSources performs live availability checks. It touches the network and
// must only be called on explicit user request.
func (a *App) CheckSources(ctx context.Context) map[string]error {
	client, err := a.httpClient()
	res := map[string]error{}
	for _, info := range sources.Catalog() {
		if err != nil {
			res[info.Name] = err
			continue
		}
		s, _ := sources.Build(info.Name, sources.Settings{APIKey: a.Cfg.APIKey(info.Name), Client: client})
		cctx, cancel := context.WithTimeout(ctx, 20*time.Second)
		res[info.Name] = s.Available(cctx)
		cancel()
	}
	return res
}

// Export writes the project in each requested format and returns the paths.
func (a *App) Export(p *models.Project, formats []string, dir string) ([]string, error) {
	if len(formats) == 0 {
		formats = p.ExportFormats
	}
	if dir == "" {
		dir = p.OutputDir
	}
	var paths []string
	for _, f := range formats {
		path, err := reporting.Write(p, f, dir)
		if err != nil {
			return paths, err
		}
		paths = append(paths, path)
	}
	return paths, nil
}
