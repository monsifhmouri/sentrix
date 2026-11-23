package sources

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"time"

	"sentrix/pkg/models"
)

type urlscan struct{ base }

func newURLScan(s Settings) Source { return &urlscan{newBase("https://urlscan.io", s)} }

func (u *urlscan) Name() string   { return "urlscan" }
func (u *urlscan) Access() Access { return AccessOptionalKey }
func (u *urlscan) Description() string {
	return "urlscan.io public search (pages previously scanned by anyone)"
}

func (u *urlscan) headers() map[string]string {
	if u.apiKey == "" {
		return nil
	}
	return map[string]string{"API-Key": u.apiKey}
}

func (u *urlscan) Available(ctx context.Context) error {
	return u.ping(ctx, u.url+"/api/v1/search/?q=domain:example.com&size=0", u.headers())
}

type urlscanResponse struct {
	Results []struct {
		ID   string `json:"_id"`
		Task struct {
			Time string `json:"time"`
			URL  string `json:"url"`
		} `json:"task"`
		Page struct {
			URL    string `json:"url"`
			Status string `json:"status"`
		} `json:"page"`
	} `json:"results"`
}

func (u *urlscan) Fetch(ctx context.Context, domain string) ([]Finding, error) {
	size := min(u.max, 10000)
	q := url.Values{}
	q.Set("q", "domain:"+domain)
	q.Set("size", fmt.Sprint(size))
	body, err := u.get(ctx, u.url+"/api/v1/search/?"+q.Encode(), u.headers())
	if err != nil {
		return nil, err
	}
	var resp urlscanResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("%w: urlscan: %v", ErrParse, err)
	}
	var out []Finding
	for _, r := range resp.Results {
		raw := r.Page.URL
		if raw == "" {
			raw = r.Task.URL
		}
		if raw == "" {
			continue
		}
		t, _ := time.Parse(time.RFC3339, r.Task.Time)
		out = append(out, Finding{
			Raw:        raw,
			Kind:       models.KindEndpoint,
			Evidence:   fmt.Sprintf("urlscan result %s, page HTTP status %s", r.ID, r.Page.Status),
			ObservedAt: t.UTC(),
		})
	}
	return out, nil
}
