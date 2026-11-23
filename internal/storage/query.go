package storage

import (
	"strings"

	"sentrix/pkg/models"
)

// Query filters records; zero-valued fields match everything.
type Query struct {
	Text   string // case-insensitive substring of the normalized URL or evidence
	Kind   models.Kind
	Class  string
	Scope  models.ScopeStatus
	Source string
}

func Filter(recs []models.Record, q Query) []models.Record {
	text := strings.ToLower(q.Text)
	var out []models.Record
	for _, r := range recs {
		if q.Kind != "" && r.Kind != q.Kind {
			continue
		}
		if q.Class != "" && r.Class != q.Class {
			continue
		}
		if q.Scope != "" && r.Scope != q.Scope {
			continue
		}
		if q.Source != "" && !contains(r.Sources(), q.Source) {
			continue
		}
		if text != "" && !matchesText(r, text) {
			continue
		}
		out = append(out, r)
	}
	return out
}

func contains(list []string, v string) bool {
	for _, x := range list {
		if x == v {
			return true
		}
	}
	return false
}

func matchesText(r models.Record, text string) bool {
	if strings.Contains(strings.ToLower(r.NormalizedURL), text) {
		return true
	}
	for _, o := range r.Observations {
		if strings.Contains(strings.ToLower(o.Evidence), text) || strings.Contains(strings.ToLower(o.OriginalURL), text) {
			return true
		}
	}
	return false
}
