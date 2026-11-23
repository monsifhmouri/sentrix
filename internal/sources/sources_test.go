package sources

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func serve(t *testing.T, h http.HandlerFunc) *httptest.Server {
	t.Helper()
	s := httptest.NewServer(h)
	t.Cleanup(s.Close)
	return s
}

func build(t *testing.T, name, url string) Source {
	t.Helper()
	s, err := Build(name, Settings{BaseURL: url, MaxResults: 100})
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestCrtshGroupsCertificatesPerName(t *testing.T) {
	srv := serve(t, func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.RawQuery, "output=json") {
			t.Errorf("missing output=json: %s", r.URL.RawQuery)
		}
		fmt.Fprint(w, `[
		 {"id":1,"name_value":"a.example.com\nwww.example.com","not_before":"2023-01-01T00:00:00"},
		 {"id":2,"name_value":"a.example.com","not_before":"2024-06-01T00:00:00"}]`)
	})
	got, err := build(t, "crtsh", srv.URL).Fetch(context.Background(), "example.com")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("want 2 grouped findings, got %d: %+v", len(got), got)
	}
	if !strings.Contains(got[0].Evidence, "2 certificate(s)") || !strings.Contains(got[0].Evidence, "2023-01-01..2024-06-01") {
		t.Errorf("evidence lost grouping detail: %s", got[0].Evidence)
	}
}

func TestWaybackSkipsHeaderRow(t *testing.T) {
	srv := serve(t, func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `[["original","timestamp","statuscode"],["https://example.com/api/v1/users","20230115123456","200"]]`)
	})
	got, err := build(t, "wayback", srv.URL).Fetch(context.Background(), "example.com")
	if err != nil || len(got) != 1 {
		t.Fatalf("got %v, %v", got, err)
	}
	if got[0].Raw != "https://example.com/api/v1/users" || got[0].ObservedAt.Year() != 2023 {
		t.Errorf("bad finding: %+v", got[0])
	}
}

func TestCommonCrawlUsesNewestIndexAndHandles404(t *testing.T) {
	var cdxHits int32
	var srv *httptest.Server
	srv = serve(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/collinfo.json":
			fmt.Fprintf(w, `[{"id":"CC-NEW","cdx-api":"%s/CC-NEW-index"},{"id":"CC-OLD","cdx-api":"%s/CC-OLD-index"}]`, srv.URL, srv.URL)
		case "/CC-NEW-index":
			atomic.AddInt32(&cdxHits, 1)
			if r.URL.Query().Get("url") == "empty.test" {
				http.Error(w, `{"message":"No Captures found"}`, 404)
				return
			}
			fmt.Fprintln(w, `{"url":"https://example.com/graphql","timestamp":"20240101000000","status":"200"}`)
			fmt.Fprintln(w, `not json`)
		default:
			t.Errorf("unexpected path %s", r.URL.Path)
		}
	})
	src := build(t, "commoncrawl", srv.URL)
	got, err := src.Fetch(context.Background(), "example.com")
	if err != nil || len(got) != 1 || !strings.Contains(got[0].Evidence, "CC-NEW") {
		t.Fatalf("got %+v, %v", got, err)
	}
	got, err = src.Fetch(context.Background(), "empty.test")
	if err != nil || len(got) != 0 {
		t.Fatalf("404 should mean no captures, got %+v, %v", got, err)
	}
}

func TestURLScanSendsKeyOnlyWhenConfigured(t *testing.T) {
	var gotKey string
	srv := serve(t, func(w http.ResponseWriter, r *http.Request) {
		gotKey = r.Header.Get("API-Key")
		fmt.Fprint(w, `{"results":[{"_id":"abc","task":{"time":"2024-02-01T10:00:00.000Z","url":"https://x.example.com/"},"page":{"url":"https://x.example.com/app.js","status":"200"}}]}`)
	})
	got, err := build(t, "urlscan", srv.URL).Fetch(context.Background(), "example.com")
	if err != nil || len(got) != 1 || got[0].Raw != "https://x.example.com/app.js" {
		t.Fatalf("got %+v, %v", got, err)
	}
	if gotKey != "" {
		t.Errorf("API key header sent without a key: %q", gotKey)
	}
	keyed, _ := Build("urlscan", Settings{BaseURL: srv.URL, APIKey: "secret"})
	if _, err := keyed.Fetch(context.Background(), "example.com"); err != nil || gotKey != "secret" {
		t.Errorf("key not sent: %q, %v", gotKey, err)
	}
}

func TestOTXReturnsPartialResultsWithError(t *testing.T) {
	srv := serve(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/url_list"):
			fmt.Fprint(w, `{"url_list":[{"url":"https://example.com/openapi.json","date":"2024-01-01T00:00:00","httpcode":200}],"has_next":false}`)
		default:
			http.Error(w, "boom", 500)
		}
	})
	got, err := build(t, "otx", srv.URL).Fetch(context.Background(), "example.com")
	if len(got) != 1 {
		t.Fatalf("partial results lost: %+v", got)
	}
	if Classify(err) != "unavailable" {
		t.Errorf("want unavailable error, got %v (%s)", err, Classify(err))
	}
}

func TestRetryAfterThenRateLimited(t *testing.T) {
	var calls int32
	srv := serve(t, func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.Header().Set("Retry-After", "0")
		http.Error(w, "slow down", 429)
	})
	_, err := build(t, "crtsh", srv.URL).Fetch(context.Background(), "example.com")
	if !errors.Is(err, ErrRateLimited) || Classify(err) != "rate_limited" {
		t.Fatalf("want ErrRateLimited, got %v", err)
	}
	if n := atomic.LoadInt32(&calls); n != maxRetries+1 {
		t.Errorf("want %d attempts, got %d", maxRetries+1, n)
	}
}

func TestErrorClassification(t *testing.T) {
	srv := serve(t, func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, `{"not":"an array"}`) })
	if _, err := build(t, "crtsh", srv.URL).Fetch(context.Background(), "example.com"); Classify(err) != "parse_error" {
		t.Errorf("bad JSON should be parse_error, got %v", err)
	}
	forbidden := serve(t, func(w http.ResponseWriter, r *http.Request) { http.Error(w, "no", 403) })
	if _, err := build(t, "crtsh", forbidden.URL).Fetch(context.Background(), "example.com"); Classify(err) != "http_error" {
		t.Errorf("403 should be http_error, got %v", err)
	}
	if err := build(t, "crtsh", forbidden.URL).Available(context.Background()); err != nil {
		t.Errorf("a 403 still proves the provider is reachable: %v", err)
	}
	dead := httptest.NewServer(http.NotFoundHandler())
	dead.Close()
	if err := build(t, "crtsh", dead.URL).Available(context.Background()); Classify(err) != "unavailable" {
		t.Errorf("closed server should be unavailable, got %v", err)
	}
}

func TestContextCancellation(t *testing.T) {
	release := make(chan struct{})
	srv := serve(t, func(w http.ResponseWriter, r *http.Request) { <-release })
	defer close(release)
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	_, err := build(t, "crtsh", srv.URL).Fetch(ctx, "example.com")
	if Classify(err) != "timeout" {
		t.Errorf("want timeout, got %v (%s)", err, Classify(err))
	}
}

func TestLimiterSpacesRequests(t *testing.T) {
	l := NewLimiter(50) // 20ms apart
	start := time.Now()
	for i := 0; i < 4; i++ {
		if err := l.Wait(context.Background()); err != nil {
			t.Fatal(err)
		}
	}
	if d := time.Since(start); d < 55*time.Millisecond {
		t.Errorf("4 requests at 50/s finished in %v, limiter not spacing", d)
	}
}

func TestCatalogMatchesAdapters(t *testing.T) {
	for _, info := range Catalog() {
		s, err := Build(info.Name, Settings{})
		if err != nil {
			t.Fatal(err)
		}
		if s.Description() != info.Description || s.Access() != info.Access || s.Name() != info.Name {
			t.Errorf("catalog entry for %s drifted from adapter", info.Name)
		}
	}
}
