// Package deduplication decides when two normalized observations are the same
// record. The mode is configurable per project.
package deduplication

import (
	"net/url"
	"strings"

	"sentrix/pkg/models"
)

type Mode string

const (
	// Strict treats URLs with different query values as different records.
	Strict Mode = "strict"
	// ParamNames collapses URLs that differ only in query parameter values,
	// which removes most historical-archive noise (?id=1, ?id=2, ...).
	ParamNames Mode = "params"
)

func ParseMode(s string) (Mode, bool) {
	switch Mode(s) {
	case Strict, ParamNames:
		return Mode(s), true
	}
	return "", false
}

// Key returns the identity of a record under the given mode.
func Key(kind models.Kind, normalized string, mode Mode) string {
	if kind == models.KindHost || mode == Strict {
		return string(kind) + "|" + normalized
	}
	u, err := url.Parse(normalized)
	if err != nil || u.RawQuery == "" {
		return string(kind) + "|" + normalized
	}
	seen := map[string]bool{}
	var names []string
	for _, part := range strings.Split(u.RawQuery, "&") {
		k, _, _ := strings.Cut(part, "=")
		if !seen[k] {
			seen[k] = true
			names = append(names, k)
		}
	}
	u.RawQuery = strings.Join(names, "&")
	return string(kind) + "|" + u.String()
}
