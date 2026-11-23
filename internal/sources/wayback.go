package sources

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"

	"sentrix/pkg/models"
)

type wayback struct{ base }

func newWayback(s Settings) Source { return &wayback{newBase("https://web.archive.org", s)} }

func (w *wayback) Name() string   { return "wayback" }
func (w *wayback) Access() Access { return AccessPublic }
func (w *wayback) Description() string {
	return "Internet Archive CDX API (historical URLs captured by the Wayback Machine)"
}

func (w *wayback) Available(ctx context.Context) error {
	return w.ping(ctx, w.url+"/cdx/search/cdx?url=archive.org&limit=1&output=json", nil)
}

func (w *wayback) Fetch(ctx context.Context, domain string) ([]Finding, error) {
	q := url.Values{}
	q.Set("url", domain)
	q.Set("matchType", "domain")
	q.Set("output", "json")
	q.Set("fl", "original,timestamp,statuscode")
	q.Set("collapse", "urlkey")
	q.Set("limit", fmt.Sprint(w.max))
	body, err := w.get(ctx, w.url+"/cdx/search/cdx?"+q.Encode(), nil)
	if err != nil {
		return nil, err
	}
	if len(body) == 0 {
		return nil, nil
	}
	var rows [][]string
	if err := json.Unmarshal(body, &rows); err != nil {
		return nil, fmt.Errorf("%w: wayback: %v", ErrParse, err)
	}
	var out []Finding
	for i, r := range rows {
		if i == 0 || len(r) < 3 { // first row is the header
			continue
		}
		ts := parseTimestamp("20060102150405", r[1])
		out = append(out, Finding{
			Raw:        r[0],
			Kind:       models.KindEndpoint,
			Evidence:   fmt.Sprintf("archived capture %s, HTTP status %s", r[1], r[2]),
			ObservedAt: ts,
		})
	}
	return out, nil
}
