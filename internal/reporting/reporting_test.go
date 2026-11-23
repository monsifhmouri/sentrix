package reporting

import (
	"encoding/csv"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"sentrix/pkg/models"
)

func project() *models.Project {
	t0 := time.Date(2026, 11, 1, 10, 0, 0, 0, time.UTC)
	return &models.Project{
		Name: "Acme Q4", Domain: "example.com", Mode: "passive", DedupeMode: "strict",
		Scope:     models.ScopeRules{Roots: []string{"example.com"}},
		CreatedAt: t0, UpdatedAt: t0,
		Runs: []models.SourceRun{{Source: "otx", Status: "rate_limited", Error: "rate limited by provider", Finished: t0},
			{Source: "wayback", Status: "ok", Findings: 2, Finished: t0}},
		Snapshots: []models.Snapshot{{At: t0, Total: 2, New: 2, InScope: 1}},
		Records: []models.Record{
			{ID: "a", Kind: models.KindEndpoint, OriginalURL: "https://example.com/<script>alert(1)</script>", NormalizedURL: "https://example.com/graphql",
				Hostname: "example.com", Path: "/graphql", Class: "graphql", ClassBasis: "inferred (medium confidence): path contains a graphql segment",
				Scope: models.InScope, ScopeReason: "within root example.com", Confidence: models.ConfidenceHigh, FirstSeen: t0,
				Observations: []models.Observation{{Source: "wayback", OriginalURL: "https://example.com/graphql"}, {Source: "otx", OriginalURL: "HTTPS://example.com/graphql/"}}},
			{ID: "b", Kind: models.KindHost, OriginalURL: "evil.test", NormalizedURL: "evil.test", Hostname: "evil.test", Class: "hostname",
				Scope: models.OutOfScope, ScopeReason: "no scope rule matches this host", Confidence: models.ConfidenceMedium, FirstSeen: t0,
				Observations: []models.Observation{{Source: "wayback", OriginalURL: "evil.test"}}},
		},
	}
}

func TestAllFormatsWriteAndAreReproducible(t *testing.T) {
	p := project()
	for _, f := range []string{"json", "csv", "txt", "html"} {
		d1, d2 := t.TempDir(), t.TempDir()
		p1, err := Write(p, f, d1)
		if err != nil {
			t.Fatalf("%s: %v", f, err)
		}
		p2, _ := Write(p, f, d2)
		a, _ := os.ReadFile(p1)
		b, _ := os.ReadFile(p2)
		if len(a) == 0 || string(a) != string(b) {
			t.Errorf("%s export empty or not reproducible", f)
		}
		if !strings.Contains(string(a), "MONSIF HMOURI") && f != "csv" {
			t.Errorf("%s export is missing developer attribution", f)
		}
	}
	if _, err := Write(p, "pdf", t.TempDir()); err == nil {
		t.Error("unknown format must fail")
	}
}

func TestJSONHasProvenanceAndNote(t *testing.T) {
	path, _ := Write(project(), "json", t.TempDir())
	data, _ := os.ReadFile(path)
	var out struct {
		Note    string
		Records []models.Record
	}
	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatal(err)
	}
	if len(out.Records) != 2 || len(out.Records[0].Observations) != 2 || !strings.Contains(out.Note, "did not contact the target") {
		t.Errorf("unexpected JSON export: %s", data)
	}
}

func TestCSVIsParseable(t *testing.T) {
	path, _ := Write(project(), "csv", t.TempDir())
	f, _ := os.Open(path)
	defer f.Close()
	rows, err := csv.NewReader(f).ReadAll()
	if err != nil || len(rows) != 3 {
		t.Fatalf("rows=%d err=%v", len(rows), err)
	}
	if rows[1][12] != "otx;wayback" {
		t.Errorf("sources column: %q", rows[1][12])
	}
}

func TestHTMLEscapesAndReportsLimitations(t *testing.T) {
	path, _ := Write(project(), "html", t.TempDir())
	data, _ := os.ReadFile(path)
	h := string(data)
	if strings.Contains(h, "<script>alert") {
		t.Error("untrusted URL was not escaped")
	}
	for _, want := range []string{"rate_limited", "inferred", "Out-of-scope records (1)", "did not contact the target", "github.com/monsifhmouri"} {
		if !strings.Contains(h, want) {
			t.Errorf("HTML missing %q", want)
		}
	}
}

func TestStats(t *testing.T) {
	s := ComputeStats(project())
	if s.Hosts != 1 || s.Endpoints != 1 || s.InScope != 1 || s.OutScope != 1 || s.MultiSource != 1 || len(s.Timeline) != 1 {
		t.Errorf("%+v", s)
	}
}
