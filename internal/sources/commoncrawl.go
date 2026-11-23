package sources

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"

	"sentrix/pkg/models"
)

type commonCrawl struct{ base }

func newCommonCrawl(s Settings) Source {
	return &commonCrawl{newBase("https://index.commoncrawl.org", s)}
}

func (c *commonCrawl) Name() string   { return "commoncrawl" }
func (c *commonCrawl) Access() Access { return AccessPublic }
func (c *commonCrawl) Description() string {
	return "Common Crawl index (URLs from the most recent crawl)"
}

func (c *commonCrawl) Available(ctx context.Context) error {
	return c.ping(ctx, c.url+"/collinfo.json", nil)
}

type ccCollection struct {
	ID     string `json:"id"`
	CDXAPI string `json:"cdx-api"`
}

type ccRow struct {
	URL       string `json:"url"`
	Timestamp string `json:"timestamp"`
	Status    string `json:"status"`
}

func (c *commonCrawl) Fetch(ctx context.Context, domain string) ([]Finding, error) {
	info, err := c.get(ctx, c.url+"/collinfo.json", nil)
	if err != nil {
		return nil, err
	}
	var cols []ccCollection
	if err := json.Unmarshal(info, &cols); err != nil || len(cols) == 0 || cols[0].CDXAPI == "" {
		return nil, fmt.Errorf("%w: commoncrawl collection list", ErrParse)
	}
	latest := cols[0] // the provider lists the newest crawl first

	q := url.Values{}
	q.Set("url", domain)
	q.Set("matchType", "domain")
	q.Set("output", "json")
	q.Set("fl", "url,timestamp,status")
	q.Set("limit", fmt.Sprint(c.max))
	body, err := c.get(ctx, latest.CDXAPI+"?"+q.Encode(), nil)
	var se *StatusError
	if errors.As(err, &se) && se.Status == 404 {
		return nil, nil // the index answers 404 when a domain has no captures
	}
	if err != nil {
		return nil, err
	}

	var out []Finding
	sc := bufio.NewScanner(bytes.NewReader(body))
	sc.Buffer(make([]byte, 0, 64*1024), 1<<20)
	for sc.Scan() {
		var r ccRow
		if json.Unmarshal(sc.Bytes(), &r) != nil || r.URL == "" {
			continue
		}
		out = append(out, Finding{
			Raw:        r.URL,
			Kind:       models.KindEndpoint,
			Evidence:   fmt.Sprintf("crawl %s capture %s, HTTP status %s", latest.ID, r.Timestamp, r.Status),
			ObservedAt: parseTimestamp("20060102150405", r.Timestamp),
		})
	}
	if err := sc.Err(); err != nil {
		return out, fmt.Errorf("%w: commoncrawl: %v", ErrParse, err)
	}
	return out, nil
}
