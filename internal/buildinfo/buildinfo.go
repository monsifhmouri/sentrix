// Package buildinfo holds product identity and build metadata. Version, Commit
// and Date are overridden at link time (see the Makefile).
package buildinfo

import "fmt"

var (
	Version = "0.1.0-dev"
	Commit  = "unknown"
	Date    = "unknown"
)

const (
	Name      = "SENTRIX"
	Subtitle  = "ADVANCED ATTACK SURFACE INTELLIGENCE"
	Developer = "MONSIF HMOURI"
	Alias     = "MONSTR MIND"
	Org       = "MONSTR MIND LABS"
	GitHub    = "https://github.com/monsifhmouri"
)

// UserAgent identifies SENTRIX to the public data sources it queries.
func UserAgent() string {
	return fmt.Sprintf("SENTRIX/%s (+%s)", Version, GitHub)
}

// Summary is the one-line version string used by `sentrix version`.
func Summary() string {
	return fmt.Sprintf("%s %s (commit %s, built %s) by %s / %s", Name, Version, Commit, Date, Developer, Org)
}
