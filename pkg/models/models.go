// Package models defines the unified result schema shared by every SENTRIX
// component: sources produce findings, the intelligence layer turns them into
// Records, storage persists Projects, and reporting renders them.
package models

import (
	"sort"
	"time"
)

type Kind string

const (
	KindHost     Kind = "host"
	KindEndpoint Kind = "endpoint"
)

type ScopeStatus string

const (
	InScope    ScopeStatus = "in-scope"
	OutOfScope ScopeStatus = "out-of-scope"
)

type Confidence string

const (
	ConfidenceMedium Confidence = "medium"
	ConfidenceHigh   Confidence = "high"
)

// Observation is one piece of provenance: a single source reporting a single
// resource. Observations are never merged away, so conflicting reports stay
// visible (for example two different original URLs that normalize identically).
type Observation struct {
	Source       string     `json:"source"`
	DiscoveredAt time.Time  `json:"discovered_at"`
	ObservedAt   *time.Time `json:"observed_at,omitempty"`
	OriginalURL  string     `json:"original"`
	Evidence     string     `json:"evidence"`
}

// Record is a discovered host or endpoint. Everything except Class is taken
// from collected data; Class is a heuristic and is labelled as such.
type Record struct {
	ID            string        `json:"id"`
	Kind          Kind          `json:"kind"`
	OriginalURL   string        `json:"original_url"`
	NormalizedURL string        `json:"normalized_url"`
	Hostname      string        `json:"hostname"`
	Path          string        `json:"path,omitempty"`
	QueryParams   []string      `json:"query_params,omitempty"`
	Class         string        `json:"class"`
	ClassBasis    string        `json:"class_basis"`
	Scope         ScopeStatus   `json:"scope"`
	ScopeReason   string        `json:"scope_reason"`
	Confidence    Confidence    `json:"confidence"`
	FirstSeen     time.Time     `json:"first_seen"`
	Observations  []Observation `json:"observations"`
}

// Sources returns the distinct source names that reported this record.
func (r Record) Sources() []string {
	seen := map[string]bool{}
	var out []string
	for _, o := range r.Observations {
		if !seen[o.Source] {
			seen[o.Source] = true
			out = append(out, o.Source)
		}
	}
	sort.Strings(out)
	return out
}

// OriginalVariants returns the distinct original spellings observed.
func (r Record) OriginalVariants() []string {
	seen := map[string]bool{}
	var out []string
	for _, o := range r.Observations {
		if !seen[o.OriginalURL] {
			seen[o.OriginalURL] = true
			out = append(out, o.OriginalURL)
		}
	}
	sort.Strings(out)
	return out
}

// ScopeRules is the user-declared authorization boundary.
type ScopeRules struct {
	Roots       []string `json:"roots"`
	Hosts       []string `json:"hosts,omitempty"`
	URLPrefixes []string `json:"url_prefixes,omitempty"`
	Exclude     []string `json:"exclude,omitempty"`
}

type SourceRun struct {
	Source   string    `json:"source"`
	Started  time.Time `json:"started"`
	Finished time.Time `json:"finished"`
	Status   string    `json:"status"`
	Findings int       `json:"findings"`
	Skipped  int       `json:"skipped"`
	Error    string    `json:"error,omitempty"`
}

// Snapshot summarises the project after one scan, for historical comparison.
type Snapshot struct {
	At        time.Time `json:"at"`
	Total     int       `json:"total"`
	New       int       `json:"new"`
	InScope   int       `json:"in_scope"`
	Hosts     int       `json:"hosts"`
	Endpoints int       `json:"endpoints"`
}

type Project struct {
	Name          string      `json:"name"`
	Domain        string      `json:"domain"`
	Program       string      `json:"program,omitempty"`
	Mode          string      `json:"mode"`
	Scope         ScopeRules  `json:"scope"`
	Sources       []string    `json:"sources"`
	Concurrency   int         `json:"concurrency"`
	RatePerSecond float64     `json:"rate_per_second"`
	DedupeMode    string      `json:"dedupe_mode"`
	OutputDir     string      `json:"output_dir"`
	ExportFormats []string    `json:"export_formats"`
	CreatedAt     time.Time   `json:"created_at"`
	UpdatedAt     time.Time   `json:"updated_at"`
	Records       []Record    `json:"records"`
	Runs          []SourceRun `json:"runs"`
	Snapshots     []Snapshot  `json:"snapshots"`
}
