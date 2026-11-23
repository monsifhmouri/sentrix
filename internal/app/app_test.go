package app

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"sentrix/internal/config"
	"sentrix/pkg/models"
)

func newApp(t *testing.T) *App {
	t.Helper()
	dir := t.TempDir()
	cfg := config.Default()
	cfg.DataDir, cfg.OutputDir = filepath.Join(dir, "p"), filepath.Join(dir, "o")
	path := filepath.Join(dir, "config.json")
	if err := cfg.Save(path); err != nil {
		t.Fatal(err)
	}
	a, err := New(path)
	if err != nil {
		t.Fatal(err)
	}
	return a
}

func TestNewProjectValidation(t *testing.T) {
	a := newApp(t)
	good := ProjectInput{Name: "Acme", Domain: "example.com", ScopeEntries: []string{"exclude:dev.example.com"}}
	p, err := a.NewProject(good)
	if err != nil {
		t.Fatal(err)
	}
	if p.Mode != ModePassive || p.Concurrency != 3 || len(p.Sources) != 5 || p.Scope.Exclude[0] != "dev.example.com" {
		t.Errorf("defaults not applied: %+v", p)
	}
	bad := []ProjectInput{
		{Name: "", Domain: "example.com"},
		{Name: "x", Domain: "not a domain"},
		{Name: "x", Domain: "example.com", Mode: "aggressive"},
		{Name: "x", Domain: "example.com", Sources: []string{"shodan"}},
		{Name: "x", Domain: "example.com", Concurrency: 100},
		{Name: "x", Domain: "example.com", RatePerSecond: 1000},
		{Name: "x", Domain: "example.com", Formats: []string{"pdf"}},
		{Name: "x", Domain: "example.com", DedupeMode: "fuzzy"},
	}
	for i, in := range bad {
		if _, err := a.NewProject(in); err == nil {
			t.Errorf("case %d should be rejected: %+v", i, in)
		}
	}
	if err := a.Store.Save(p); err != nil {
		t.Fatal(err)
	}
	if _, err := a.NewProject(good); err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Errorf("duplicate name accepted: %v", err)
	}
}

// End to end: real adapter, pipeline, storage and exports against a local
// mock of the provider. No third-party host is contacted.
func TestScanEndToEndAgainstMockProvider(t *testing.T) {
	mock := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/cdx/search") {
			fmt.Fprint(w, `[["original","timestamp","statuscode"],["https://example.com/api/v1/users?id=1","20240101000000","200"],["https://other.test/x","20240101000000","200"]]`)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer mock.Close()

	a := newApp(t)
	a.baseURLs = map[string]string{"wayback": mock.URL}
	p, err := a.NewProject(ProjectInput{Name: "e2e", Domain: "example.com", Sources: []string{"wayback"}})
	if err != nil {
		t.Fatal(err)
	}
	sum, err := a.Scan(context.Background(), p, nil)
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	if sum.Total != 4 { // 2 endpoints + their 2 hosts
		t.Fatalf("total = %d, want 4: %+v", sum.Total, p.Records)
	}
	var inScope, outScope int
	for _, r := range p.Records {
		if r.Scope == models.InScope {
			inScope++
		} else {
			outScope++
		}
	}
	if inScope != 2 || outScope != 2 {
		t.Errorf("scope labelling: in=%d out=%d", inScope, outScope)
	}

	// A second scan must be incremental: nothing new, history grows.
	sum, err = a.Scan(context.Background(), p, nil)
	if err != nil || sum.New != 0 || len(p.Snapshots) != 2 {
		t.Errorf("rescan: new=%d snapshots=%d err=%v", sum.New, len(p.Snapshots), err)
	}

	loaded, err := a.Store.Load("e2e")
	if err != nil || len(loaded.Records) != 4 {
		t.Fatalf("scan result not persisted: %v", err)
	}
	paths, err := a.Export(loaded, []string{"json", "csv", "txt", "html"}, "")
	if err != nil || len(paths) != 4 {
		t.Fatalf("export: %v %v", paths, err)
	}
}
