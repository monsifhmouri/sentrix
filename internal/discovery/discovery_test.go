package discovery

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"sentrix/internal/deduplication"
	"sentrix/internal/scope"
	"sentrix/internal/sources"
	"sentrix/pkg/models"
)

type fake struct {
	name     string
	findings []sources.Finding
	availErr error
	fetchErr error
	delay    time.Duration
	active   *int32
	peak     *int32
}

func (f fake) Name() string        { return f.name }
func (f fake) Description() string { return "fake" }
func (f fake) Access() sources.Access {
	return sources.AccessPublic
}
func (f fake) Available(context.Context) error { return f.availErr }
func (f fake) Fetch(ctx context.Context, domain string) ([]sources.Finding, error) {
	if f.active != nil {
		n := atomic.AddInt32(f.active, 1)
		for {
			p := atomic.LoadInt32(f.peak)
			if n <= p || atomic.CompareAndSwapInt32(f.peak, p, n) {
				break
			}
		}
		defer atomic.AddInt32(f.active, -1)
	}
	select {
	case <-time.After(f.delay):
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	return f.findings, f.fetchErr
}

func project(t *testing.T) *models.Project {
	t.Helper()
	rules, err := scope.ParseRules("example.com", nil)
	if err != nil {
		t.Fatal(err)
	}
	return &models.Project{Name: "t", Domain: "example.com", Scope: rules}
}

func url(s string) sources.Finding {
	return sources.Finding{Raw: s, Kind: models.KindEndpoint, Evidence: "e"}
}

func TestRunMergesSourcesAndReportsFailures(t *testing.T) {
	p := project(t)
	srcs := []sources.Source{
		fake{name: "a", findings: []sources.Finding{url("https://example.com/x"), url("ftp://bad")}},
		fake{name: "b", findings: []sources.Finding{url("https://example.com/x")}},
		fake{name: "down", availErr: fmt.Errorf("%w: refused", sources.ErrUnavailable)},
		fake{name: "partial", findings: []sources.Finding{url("https://example.com/y")}, fetchErr: sources.ErrRateLimited},
	}
	var events []Event
	sum, err := Run(context.Background(), p, srcs, Options{Concurrency: 2, Dedupe: deduplication.Strict,
		Notify: func(e Event) { events = append(events, e) }})
	if err != nil {
		t.Fatal(err)
	}
	status := map[string]models.SourceRun{}
	for _, r := range sum.Runs {
		status[r.Source] = r
	}
	if status["down"].Status != "unavailable" || status["partial"].Status != "rate_limited" || status["a"].Status != "ok" {
		t.Errorf("unexpected statuses: %+v", status)
	}
	if status["a"].Skipped != 1 {
		t.Errorf("unparseable finding not counted: %+v", status["a"])
	}
	// partial results from the rate-limited source are kept
	var haveY bool
	for _, r := range p.Records {
		if r.NormalizedURL == "https://example.com/y" {
			haveY = true
		}
	}
	if !haveY {
		t.Error("partial results were discarded")
	}
	if len(p.Snapshots) != 1 || p.Snapshots[0].New != sum.New || len(p.Runs) != 4 {
		t.Errorf("history not recorded: %+v", p.Snapshots)
	}
	if len(events) == 0 {
		t.Error("no progress events")
	}
}

func TestConcurrencyIsBounded(t *testing.T) {
	var active, peak int32
	var srcs []sources.Source
	for i := 0; i < 8; i++ {
		srcs = append(srcs, fake{name: fmt.Sprint("s", i), delay: 20 * time.Millisecond, active: &active, peak: &peak})
	}
	if _, err := Run(context.Background(), project(t), srcs, Options{Concurrency: 3}); err != nil {
		t.Fatal(err)
	}
	if p := atomic.LoadInt32(&peak); p > 3 || p < 2 {
		t.Errorf("peak concurrency %d, want 2..3", p)
	}
}

func TestCancellationKeepsPartialWork(t *testing.T) {
	p := project(t)
	ctx, cancel := context.WithCancel(context.Background())
	srcs := []sources.Source{
		fake{name: "fast", findings: []sources.Finding{url("https://example.com/kept")}},
		fake{name: "slow", delay: time.Minute},
	}
	var once sync.Once
	_, err := Run(ctx, p, srcs, Options{Concurrency: 2, Notify: func(e Event) {
		if e.Source == "fast" && e.Phase == "done" {
			once.Do(cancel)
		}
	}})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("want context.Canceled, got %v", err)
	}
	var slowStatus string
	for _, r := range p.Runs {
		if r.Source == "slow" {
			slowStatus = r.Status
		}
	}
	if slowStatus != "canceled" || len(p.Records) == 0 {
		t.Errorf("slow=%q records=%d", slowStatus, len(p.Records))
	}
}

func TestPerSourceTimeout(t *testing.T) {
	p := project(t)
	_, _ = Run(context.Background(), p, []sources.Source{fake{name: "slow", delay: time.Minute}},
		Options{Concurrency: 1, Timeout: 30 * time.Millisecond})
	if len(p.Runs) != 1 || p.Runs[0].Status != "timeout" {
		t.Errorf("runs: %+v", p.Runs)
	}
}
