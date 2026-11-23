package intelligence

import (
	"testing"
	"time"

	"sentrix/internal/deduplication"
	"sentrix/internal/scope"
	"sentrix/internal/sources"
	"sentrix/pkg/models"
)

var now = time.Date(2026, 11, 23, 12, 0, 0, 0, time.UTC)

func newAgg(t *testing.T, mode deduplication.Mode, existing []models.Record) *Aggregator {
	t.Helper()
	rules, err := scope.ParseRules("example.com", []string{"exclude:dev.example.com"})
	if err != nil {
		t.Fatal(err)
	}
	g, err := scope.New(rules)
	if err != nil {
		t.Fatal(err)
	}
	return New(g, mode, existing)
}

func ep(raw string) sources.Finding {
	return sources.Finding{Raw: raw, Kind: models.KindEndpoint, Evidence: "test"}
}

func find(recs []models.Record, kind models.Kind, norm string) *models.Record {
	for i := range recs {
		if recs[i].Kind == kind && recs[i].NormalizedURL == norm {
			return &recs[i]
		}
	}
	return nil
}

func TestProvenanceAndConfidence(t *testing.T) {
	a := newAgg(t, deduplication.Strict, nil)
	// Two spellings that normalize identically: both must be preserved.
	must(t, a.Add("wayback", ep("HTTPS://Example.com/api/v1/users/"), now))
	must(t, a.Add("urlscan", ep("https://example.com:443/api/v1/users"), now))
	r := find(a.Records(), models.KindEndpoint, "https://example.com/api/v1/users")
	if r == nil {
		t.Fatal("record missing")
	}
	if len(r.Observations) != 2 || len(r.OriginalVariants()) != 2 {
		t.Errorf("conflicting observations were merged away: %+v", r.Observations)
	}
	if r.Confidence != models.ConfidenceHigh {
		t.Errorf("two sources should give high confidence, got %s", r.Confidence)
	}
	if r.Class != "rest-api" || r.ClassBasis == "" || r.Scope != models.InScope {
		t.Errorf("bad classification/scope: %+v", r)
	}
	if len(r.ID) != 12 {
		t.Errorf("unexpected ID %q", r.ID)
	}
}

func TestSingleSourceIsMedium(t *testing.T) {
	a := newAgg(t, deduplication.Strict, nil)
	must(t, a.Add("wayback", ep("https://example.com/about"), now))
	if r := find(a.Records(), models.KindEndpoint, "https://example.com/about"); r.Confidence != models.ConfidenceMedium {
		t.Errorf("want medium, got %s", r.Confidence)
	}
}

func TestScopeLabelling(t *testing.T) {
	a := newAgg(t, deduplication.Strict, nil)
	must(t, a.Add("wayback", ep("https://evil.test/x"), now))
	must(t, a.Add("wayback", ep("https://dev.example.com/x"), now))
	must(t, a.Add("crtsh", sources.Finding{Raw: "mail.example.com", Kind: models.KindHost}, now))
	recs := a.Records()
	for norm, want := range map[string]models.ScopeStatus{
		"https://evil.test/x": models.OutOfScope, "https://dev.example.com/x": models.OutOfScope, "mail.example.com": models.InScope,
	} {
		r := find(recs, kindOf(norm), norm)
		if r == nil || r.Scope != want || r.ScopeReason == "" {
			t.Errorf("%s: %+v, want %s", norm, r, want)
		}
	}
}

func kindOf(norm string) models.Kind {
	if len(norm) > 4 && norm[:4] == "http" {
		return models.KindEndpoint
	}
	return models.KindHost
}

func TestDedupeModes(t *testing.T) {
	strict := newAgg(t, deduplication.Strict, nil)
	params := newAgg(t, deduplication.ParamNames, nil)
	for _, a := range []*Aggregator{strict, params} {
		must(t, a.Add("wayback", ep("https://example.com/item?id=1"), now))
		must(t, a.Add("wayback", ep("https://example.com/item?id=2"), now))
	}
	count := func(a *Aggregator) int {
		n := 0
		for _, r := range a.Records() {
			if r.Kind == models.KindEndpoint {
				n++
			}
		}
		return n
	}
	if count(strict) != 2 || count(params) != 1 {
		t.Errorf("strict=%d (want 2), params=%d (want 1)", count(strict), count(params))
	}
}

func TestIncrementalAndRescope(t *testing.T) {
	first := newAgg(t, deduplication.Strict, nil)
	must(t, first.Add("wayback", ep("https://example.com/a"), now))
	saved := first.Records()

	second := newAgg(t, deduplication.Strict, saved)
	if second.NewRecords() != 0 {
		t.Fatal("seeding must not count as new")
	}
	must(t, second.Add("wayback", ep("https://example.com/a"), now.Add(time.Hour))) // same observation
	must(t, second.Add("otx", ep("https://example.com/a"), now.Add(time.Hour)))
	must(t, second.Add("otx", ep("https://example.com/b"), now.Add(time.Hour)))
	recs := second.Records()
	a := find(recs, models.KindEndpoint, "https://example.com/a")
	if len(a.Observations) != 2 || !a.FirstSeen.Equal(now) {
		t.Errorf("history not preserved: %+v", a)
	}
	// 2 new: /b plus nothing else (host already known).
	if second.NewRecords() != 1 {
		t.Errorf("new = %d, want 1", second.NewRecords())
	}
}

func TestUnparseableFindingsReturnErrors(t *testing.T) {
	a := newAgg(t, deduplication.Strict, nil)
	if err := a.Add("x", ep("ftp://example.com/x"), now); err == nil {
		t.Error("want error for unsupported scheme")
	}
	if err := a.Add("x", sources.Finding{Raw: "not a host", Kind: models.KindHost}, now); err == nil {
		t.Error("want error for bad hostname")
	}
}

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}
