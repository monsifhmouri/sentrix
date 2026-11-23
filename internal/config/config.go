// Package config loads and saves the user configuration. Credentials may come
// from the file or from SENTRIX_<SOURCE>_API_KEY environment variables; the
// environment wins so secrets never have to touch disk.
package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

type SourceConfig struct {
	Enabled        bool    `json:"enabled"`
	APIKey         string  `json:"api_key,omitempty"`
	RatePerSecond  float64 `json:"rate_per_second"`
	TimeoutSeconds int     `json:"timeout_seconds"`
}

type Config struct {
	DataDir       string                  `json:"data_dir"`
	OutputDir     string                  `json:"output_dir"`
	Concurrency   int                     `json:"concurrency"`
	RatePerSecond float64                 `json:"rate_per_second"`
	Formats       []string                `json:"export_formats"`
	LogLevel      string                  `json:"log_level"`
	Theme         string                  `json:"theme"` // "auto" or "mono"
	Animation     bool                    `json:"startup_animation"`
	Proxy         string                  `json:"proxy,omitempty"`
	Sources       map[string]SourceConfig `json:"sources"`
}

// AllFormats are the supported export formats.
var AllFormats = []string{"json", "csv", "txt", "html"}

func Default() Config {
	home, _ := os.UserHomeDir()
	base := filepath.Join(home, ".sentrix")
	sc := func(rate float64, timeout int) SourceConfig {
		return SourceConfig{Enabled: true, RatePerSecond: rate, TimeoutSeconds: timeout}
	}
	return Config{
		DataDir:       filepath.Join(base, "projects"),
		OutputDir:     filepath.Join(base, "exports"),
		Concurrency:   3,
		RatePerSecond: 1,
		Formats:       []string{"json", "html"},
		LogLevel:      "warn",
		Theme:         "auto",
		Animation:     true,
		Sources: map[string]SourceConfig{
			"crtsh":       sc(1, 120),
			"wayback":     sc(1, 120),
			"commoncrawl": sc(1, 120),
			"urlscan":     sc(0.5, 60),
			"otx":         sc(1, 60),
		},
	}
}

func DefaultPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".sentrix", "config.json")
}

// Load reads the file at path; a missing file yields the defaults. The file
// is decoded on top of the defaults, so any field it omits keeps its default,
// including individual fields of a source entry.
func Load(path string) (Config, error) {
	c := Default()
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return c, nil
	}
	if err != nil {
		return c, err
	}
	var perSource struct {
		Sources map[string]json.RawMessage `json:"sources"`
	}
	if err := json.Unmarshal(data, &perSource); err != nil {
		return c, fmt.Errorf("parse %s: %w", path, err)
	}
	defaults := c.Sources
	c.Sources = map[string]SourceConfig{}
	for name, def := range defaults {
		c.Sources[name] = def
	}
	// Decode everything except the sources map straight onto the defaults.
	var rest map[string]json.RawMessage
	_ = json.Unmarshal(data, &rest)
	delete(rest, "sources")
	flat, _ := json.Marshal(rest)
	if err := json.Unmarshal(flat, &c); err != nil {
		return c, fmt.Errorf("parse %s: %w", path, err)
	}
	for name, raw := range perSource.Sources {
		sc := c.Sources[name]
		if err := json.Unmarshal(raw, &sc); err != nil {
			return c, fmt.Errorf("parse %s: source %q: %w", path, name, err)
		}
		c.Sources[name] = sc
	}
	c.DataDir, c.OutputDir = ExpandHome(c.DataDir), ExpandHome(c.OutputDir)
	return c, c.Validate()
}

// ExpandHome replaces a leading "~" with the user's home directory.
func ExpandHome(p string) string {
	if p == "~" || strings.HasPrefix(p, "~/") || strings.HasPrefix(p, `~\`) {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, p[1:])
		}
	}
	return p
}

func (c Config) Validate() error {
	if c.Concurrency < 1 || c.Concurrency > 32 {
		return fmt.Errorf("concurrency must be between 1 and 32")
	}
	if c.RatePerSecond < 0 || c.RatePerSecond > 50 {
		return fmt.Errorf("rate_per_second must be between 0 and 50")
	}
	switch c.LogLevel {
	case "debug", "info", "warn", "error":
	default:
		return fmt.Errorf("log_level must be debug, info, warn or error")
	}
	if c.Theme != "auto" && c.Theme != "mono" {
		return fmt.Errorf("theme must be auto or mono")
	}
	for _, f := range c.Formats {
		if !validFormat(f) {
			return fmt.Errorf("unknown export format %q", f)
		}
	}
	if c.Proxy != "" {
		u, err := url.Parse(c.Proxy)
		if err != nil || u.Host == "" {
			return fmt.Errorf("proxy must be a URL such as http://127.0.0.1:8080")
		}
	}
	return nil
}

func validFormat(f string) bool {
	for _, x := range AllFormats {
		if x == f {
			return true
		}
	}
	return false
}

// ValidFormats reports the first invalid format in list, if any.
func ValidFormats(list []string) error {
	for _, f := range list {
		if !validFormat(f) {
			return fmt.Errorf("unknown export format %q (supported: %s)", f, strings.Join(AllFormats, ", "))
		}
	}
	return nil
}

func (c Config) Save(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0o600)
}

// APIKey returns the credential for a source: environment first, then file.
func (c Config) APIKey(source string) string {
	if v := os.Getenv("SENTRIX_" + strings.ToUpper(source) + "_API_KEY"); v != "" {
		return v
	}
	return c.Sources[source].APIKey
}

// Redacted returns a copy that is safe to print: keys are masked.
func (c Config) Redacted() Config {
	out := c
	out.Sources = map[string]SourceConfig{}
	for n, s := range c.Sources {
		if s.APIKey != "" {
			s.APIKey = "********"
		}
		out.Sources[n] = s
	}
	return out
}
