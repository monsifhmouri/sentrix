// Package endpoints normalizes publicly observed URLs and classifies them.
package endpoints

import (
	"fmt"
	"net"
	"net/url"
	"path"
	"sort"
	"strings"
	"unicode"

	"sentrix/internal/scope"
)

// Parsed is the normalized view of a URL. Original is preserved untouched.
type Parsed struct {
	Original   string
	Normalized string
	Scheme     string
	Host       string
	Path       string
	Params     []string // distinct query parameter names, sorted
}

// Normalize canonicalizes scheme and host case, default ports, dot segments,
// percent-encoding, trailing slashes and query ordering, and drops fragments.
// Query values are kept; use the deduplication package to ignore them.
func Normalize(raw string) (Parsed, error) {
	orig := raw
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return Parsed{}, fmt.Errorf("empty URL")
	}
	for _, r := range raw {
		if unicode.IsControl(r) || unicode.IsSpace(r) {
			return Parsed{}, fmt.Errorf("URL contains whitespace or control characters")
		}
	}
	if !strings.Contains(raw, "://") {
		raw = "http://" + strings.TrimPrefix(raw, "//")
	}
	u, err := url.Parse(raw)
	if err != nil {
		return Parsed{}, fmt.Errorf("parse URL: %w", err)
	}
	scheme := strings.ToLower(u.Scheme)
	if scheme != "http" && scheme != "https" {
		return Parsed{}, fmt.Errorf("unsupported scheme %q", u.Scheme)
	}
	host, _, err := scope.NormalizeHost(u.Hostname())
	if err != nil {
		return Parsed{}, err
	}
	hostport := host
	if net.ParseIP(host) != nil && strings.Contains(host, ":") {
		hostport = "[" + host + "]"
	}
	if p := u.Port(); p != "" && !(scheme == "http" && p == "80") && !(scheme == "https" && p == "443") {
		hostport += ":" + p
	}

	p := normalizePath(u)
	pairs, names := normalizeQuery(u.RawQuery)

	var b strings.Builder
	b.WriteString(scheme + "://" + hostport + p)
	if len(pairs) > 0 {
		b.WriteString("?" + strings.Join(pairs, "&"))
	}
	return Parsed{Original: orig, Normalized: b.String(), Scheme: scheme, Host: hostport, Path: p, Params: names}, nil
}

// normalizePath resolves dot segments and re-encodes consistently. Paths that
// contain an encoded slash are left structurally intact, because decoding %2F
// would change which resource the path refers to.
func normalizePath(u *url.URL) string {
	esc := u.EscapedPath()
	if esc == "" {
		return "/"
	}
	if strings.Contains(strings.ToLower(esc), "%2f") {
		if len(esc) > 1 {
			esc = strings.TrimRight(esc, "/")
		}
		return esc
	}
	clean := path.Clean(u.Path)
	if !strings.HasPrefix(clean, "/") {
		clean = "/" + clean
	}
	return (&url.URL{Path: clean}).EscapedPath()
}

func normalizeQuery(rawQuery string) (pairs, names []string) {
	if rawQuery == "" {
		return nil, nil
	}
	type kv struct {
		k, v string
		hasV bool
	}
	var items []kv
	seen := map[string]bool{}
	for _, part := range strings.Split(rawQuery, "&") {
		if part == "" {
			continue
		}
		k, v, hasV := strings.Cut(part, "=")
		if dk, err := url.QueryUnescape(k); err == nil {
			k = dk
		}
		if dv, err := url.QueryUnescape(v); err == nil {
			v = dv
		}
		items = append(items, kv{k, v, hasV})
		if !seen[k] {
			seen[k] = true
			names = append(names, k)
		}
	}
	sort.SliceStable(items, func(i, j int) bool {
		if items[i].k != items[j].k {
			return items[i].k < items[j].k
		}
		return items[i].v < items[j].v
	})
	sort.Strings(names)
	for _, it := range items {
		s := url.QueryEscape(it.k)
		if it.hasV {
			s += "=" + url.QueryEscape(it.v)
		}
		pairs = append(pairs, s)
	}
	return pairs, names
}
