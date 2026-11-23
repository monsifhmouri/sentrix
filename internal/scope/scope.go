// Package scope implements the Scope Guard: the single place that decides
// whether a discovered host or URL falls inside the authorization the user
// declared. Exclusions always win over inclusions.
package scope

import (
	"fmt"
	"net"
	"net/url"
	"regexp"
	"strings"

	"sentrix/pkg/models"
)

var labelRe = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?$`)

// Verdict is the outcome of a scope check, with a human-readable reason that
// is stored next to the record for auditability.
type Verdict struct {
	InScope bool
	Reason  string
}

type prefix struct {
	host string
	path string // no trailing slash; empty matches the whole host
	raw  string
}

// Guard evaluates hosts and URLs against a set of rules.
type Guard struct {
	roots, hosts []string
	prefixes     []prefix
	exclHosts    []string
	exclPrefixes []prefix
}

// NormalizeHost lowercases a hostname, strips a trailing dot and a leading
// "*." wildcard marker. It reports whether a wildcard marker was present.
func NormalizeHost(s string) (host string, wildcard bool, err error) {
	h := strings.ToLower(strings.TrimSpace(s))
	h = strings.TrimSuffix(h, ".")
	if strings.HasPrefix(h, "*.") {
		wildcard = true
		h = h[2:]
	}
	if h == "" {
		return "", false, fmt.Errorf("empty hostname")
	}
	if ip := net.ParseIP(h); ip != nil {
		return ip.String(), wildcard, nil
	}
	if len(h) > 253 {
		return "", false, fmt.Errorf("hostname %q is longer than 253 characters", s)
	}
	for _, l := range strings.Split(h, ".") {
		if !labelRe.MatchString(l) {
			return "", false, fmt.Errorf("invalid hostname %q", s)
		}
	}
	return h, wildcard, nil
}

func normalizeRoot(s string) (string, error) {
	h, _, err := NormalizeHost(s)
	if err != nil {
		return "", err
	}
	if net.ParseIP(h) != nil {
		return "", fmt.Errorf("root scope %q must be a domain, not an IP address", s)
	}
	if !strings.Contains(h, ".") {
		return "", fmt.Errorf("root scope %q must contain at least two labels", s)
	}
	return h, nil
}

func parsePrefix(s string) (prefix, error) {
	u, err := url.Parse(strings.TrimSpace(s))
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Hostname() == "" {
		return prefix{}, fmt.Errorf("invalid URL prefix %q (expected http:// or https:// URL)", s)
	}
	h, _, err := NormalizeHost(u.Hostname())
	if err != nil {
		return prefix{}, err
	}
	p := strings.TrimRight(u.EscapedPath(), "/")
	return prefix{host: h, path: p, raw: s}, nil
}

// New validates the rules and builds a Guard.
func New(r models.ScopeRules) (*Guard, error) {
	g := &Guard{}
	for _, s := range r.Roots {
		h, err := normalizeRoot(s)
		if err != nil {
			return nil, err
		}
		g.roots = append(g.roots, h)
	}
	for _, s := range r.Hosts {
		h, _, err := NormalizeHost(s)
		if err != nil {
			return nil, err
		}
		g.hosts = append(g.hosts, h)
	}
	for _, s := range r.URLPrefixes {
		p, err := parsePrefix(s)
		if err != nil {
			return nil, err
		}
		g.prefixes = append(g.prefixes, p)
	}
	for _, s := range r.Exclude {
		if strings.Contains(s, "://") {
			p, err := parsePrefix(s)
			if err != nil {
				return nil, err
			}
			g.exclPrefixes = append(g.exclPrefixes, p)
			continue
		}
		h, _, err := NormalizeHost(s)
		if err != nil {
			return nil, err
		}
		g.exclHosts = append(g.exclHosts, h)
	}
	if len(g.roots)+len(g.hosts)+len(g.prefixes) == 0 {
		return nil, fmt.Errorf("scope is empty: define at least one root, host or URL prefix")
	}
	return g, nil
}

// ParseRules turns the target domain plus user-supplied entries into rules.
// Entries may be prefixed with root:, host:, url: or exclude:. Unprefixed
// entries are treated as a URL prefix when they contain "://" and as a root
// domain (covering its subdomains) otherwise.
func ParseRules(domain string, entries []string) (models.ScopeRules, error) {
	root, err := normalizeRoot(domain)
	if err != nil {
		return models.ScopeRules{}, fmt.Errorf("target domain: %w", err)
	}
	r := models.ScopeRules{Roots: []string{root}}
	for _, e := range entries {
		e = strings.TrimSpace(e)
		if e == "" {
			continue
		}
		kind, val := "", e
		if i := strings.Index(e, ":"); i > 0 && !strings.HasPrefix(e[i:], "://") {
			switch strings.ToLower(e[:i]) {
			case "root", "host", "url", "exclude":
				kind, val = strings.ToLower(e[:i]), strings.TrimSpace(e[i+1:])
			}
		}
		if kind == "" {
			kind = "root"
			if strings.Contains(e, "://") {
				kind = "url"
			}
		}
		switch kind {
		case "root":
			h, err := normalizeRoot(val)
			if err != nil {
				return r, err
			}
			r.Roots = appendUnique(r.Roots, h)
		case "host":
			h, _, err := NormalizeHost(val)
			if err != nil {
				return r, err
			}
			r.Hosts = appendUnique(r.Hosts, h)
		case "url":
			if _, err := parsePrefix(val); err != nil {
				return r, err
			}
			r.URLPrefixes = appendUnique(r.URLPrefixes, val)
		case "exclude":
			if !strings.Contains(val, "://") {
				h, _, err := NormalizeHost(val)
				if err != nil {
					return r, err
				}
				val = h
			} else if _, err := parsePrefix(val); err != nil {
				return r, err
			}
			r.Exclude = appendUnique(r.Exclude, val)
		}
	}
	if _, err := New(r); err != nil {
		return r, err
	}
	return r, nil
}

func appendUnique(list []string, v string) []string {
	for _, x := range list {
		if x == v {
			return list
		}
	}
	return append(list, v)
}

func hostUnder(host, root string) bool {
	return host == root || strings.HasSuffix(host, "."+root)
}

func (p prefix) matches(host, path string) bool {
	if p.host != host {
		return false
	}
	return p.path == "" || path == p.path || strings.HasPrefix(path, p.path+"/")
}

func (g *Guard) excludedHost(host string) (string, bool) {
	for _, x := range g.exclHosts {
		if hostUnder(host, x) {
			return x, true
		}
	}
	return "", false
}

// CheckHost evaluates a bare hostname. URL-prefix rules never authorize a
// whole host, so they are ignored here.
func (g *Guard) CheckHost(host string) Verdict {
	h, _, err := NormalizeHost(host)
	if err != nil {
		return Verdict{false, "not a valid hostname"}
	}
	if x, ok := g.excludedHost(h); ok {
		return Verdict{false, "excluded host rule " + x}
	}
	for _, r := range g.roots {
		if hostUnder(h, r) {
			return Verdict{true, "within root " + r}
		}
	}
	for _, x := range g.hosts {
		if h == x {
			return Verdict{true, "exact host " + x}
		}
	}
	return Verdict{false, "no scope rule matches this host"}
}

// CheckURL evaluates a URL (ideally already normalized).
func (g *Guard) CheckURL(raw string) Verdict {
	u, err := url.Parse(raw)
	if err != nil || u.Hostname() == "" {
		return Verdict{false, "unparseable URL"}
	}
	h, _, err := NormalizeHost(u.Hostname())
	if err != nil {
		return Verdict{false, "not a valid hostname"}
	}
	path := strings.TrimRight(u.EscapedPath(), "/")
	for _, p := range g.exclPrefixes {
		if p.matches(h, path) {
			return Verdict{false, "excluded URL prefix " + p.raw}
		}
	}
	if v := g.CheckHost(h); v.InScope || strings.HasPrefix(v.Reason, "excluded") {
		return v
	}
	for _, p := range g.prefixes {
		if p.matches(h, path) {
			return Verdict{true, "URL prefix " + p.raw}
		}
	}
	return Verdict{false, "no scope rule matches this URL"}
}
