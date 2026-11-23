package sources

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"sort"
	"strings"
	"time"

	"sentrix/pkg/models"
)

type crtsh struct{ base }

func newCrtsh(s Settings) Source { return &crtsh{newBase("https://crt.sh", s)} }

func (c *crtsh) Name() string   { return "crtsh" }
func (c *crtsh) Access() Access { return AccessPublic }
func (c *crtsh) Description() string {
	return "Certificate Transparency log search (hostnames from issued certificates)"
}

func (c *crtsh) Available(ctx context.Context) error { return c.ping(ctx, c.url+"/", nil) }

type crtEntry struct {
	ID        int64  `json:"id"`
	NameValue string `json:"name_value"`
	NotBefore string `json:"not_before"`
}

type crtAgg struct {
	count           int
	first, last     time.Time
	exampleID       int64
	viaWildcardCert bool
}

func (c *crtsh) Fetch(ctx context.Context, domain string) ([]Finding, error) {
	body, err := c.get(ctx, c.url+"/?q="+url.QueryEscape("%."+domain)+"&output=json", nil)
	if err != nil {
		return nil, err
	}
	if len(strings.TrimSpace(string(body))) == 0 {
		return nil, nil
	}
	var entries []crtEntry
	if err := json.Unmarshal(body, &entries); err != nil {
		return nil, fmt.Errorf("%w: crt.sh: %v", ErrParse, err)
	}

	// crt.sh returns one row per certificate, so popular hosts appear
	// hundreds of times. Rows are grouped per name here, and the count and
	// date range are recorded in the evidence rather than dropped.
	groups := map[string]*crtAgg{}
	for _, e := range entries {
		nb := parseTimestamp("2006-01-02T15:04:05", e.NotBefore)
		for _, name := range strings.Split(e.NameValue, "\n") {
			name = strings.ToLower(strings.TrimSpace(name))
			if name == "" {
				continue
			}
			g := groups[name]
			if g == nil {
				g = &crtAgg{exampleID: e.ID}
				groups[name] = g
			}
			g.count++
			if !nb.IsZero() && (g.first.IsZero() || nb.Before(g.first)) {
				g.first = nb
			}
			if !nb.IsZero() && nb.After(g.last) {
				g.last = nb
			}
		}
	}
	names := make([]string, 0, len(groups))
	for n := range groups {
		names = append(names, n)
	}
	sort.Strings(names)

	var out []Finding
	for _, n := range names {
		g := groups[n]
		ev := fmt.Sprintf("named in %d certificate(s); example crt.sh id %d", g.count, g.exampleID)
		if !g.first.IsZero() {
			ev += fmt.Sprintf("; not_before range %s..%s", g.first.Format("2006-01-02"), g.last.Format("2006-01-02"))
		}
		out = append(out, Finding{Raw: n, Kind: models.KindHost, Evidence: ev, ObservedAt: g.first})
		if len(out) >= c.max {
			break
		}
	}
	return out, nil
}
