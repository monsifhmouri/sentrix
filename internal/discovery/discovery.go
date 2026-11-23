// Package discovery runs the configured sources against a project with
// bounded concurrency and folds their findings into the project's records.
package discovery

import (
	"context"
	"sync"
	"time"

	"sentrix/internal/deduplication"
	"sentrix/internal/intelligence"
	"sentrix/internal/scope"
	"sentrix/internal/sources"
	"sentrix/pkg/models"
)

// Event reports live progress. Phase is one of: checking, fetching, done.
// Done events carry the final status ("ok", "unavailable", "timeout", ...).
type Event struct {
	Source string
	Phase  string
	Status string
	Count  int
	Err    error
}

type Options struct {
	Concurrency int
	Timeout     time.Duration // per source, covering availability check and fetch
	Dedupe      deduplication.Mode
	Notify      func(Event) // optional; calls are serialized
}

type Summary struct {
	New   int
	Total int
	Runs  []models.SourceRun
}

type outcome struct {
	run      models.SourceRun
	findings []sources.Finding
}

// Run executes srcs for p.Domain and updates p in place (records, run
// history and a snapshot). Partial results survive cancellation.
func Run(ctx context.Context, p *models.Project, srcs []sources.Source, opts Options) (Summary, error) {
	guard, err := scope.New(p.Scope)
	if err != nil {
		return Summary{}, err
	}
	agg := intelligence.New(guard, opts.Dedupe, p.Records)

	var notifyMu sync.Mutex
	notify := func(e Event) {
		if opts.Notify == nil {
			return
		}
		notifyMu.Lock()
		defer notifyMu.Unlock()
		opts.Notify(e)
	}

	workers := max(opts.Concurrency, 1)
	sem := make(chan struct{}, workers)
	results := make(chan outcome)
	var wg sync.WaitGroup

	for _, s := range srcs {
		wg.Add(1)
		go func(s sources.Source) {
			defer wg.Done()
			select {
			case sem <- struct{}{}:
				defer func() { <-sem }()
			case <-ctx.Done():
				results <- outcome{run: finish(models.SourceRun{Source: s.Name(), Started: time.Now()}, ctx.Err(), 0)}
				return
			}
			results <- fetchOne(ctx, s, p.Domain, opts.Timeout, notify)
		}(s)
	}
	go func() { wg.Wait(); close(results) }()

	now := time.Now().UTC()
	var runs []models.SourceRun
	for o := range results {
		for _, f := range o.findings {
			if err := agg.Add(o.run.Source, f, now); err != nil {
				o.run.Skipped++
			}
		}
		runs = append(runs, o.run)
		notify(Event{Source: o.run.Source, Phase: "done", Status: o.run.Status, Count: o.run.Findings})
	}

	p.Records = agg.Records()
	p.Runs = append(p.Runs, runs...)
	p.UpdatedAt = now
	snap := models.Snapshot{At: now, Total: len(p.Records), New: agg.NewRecords()}
	for _, r := range p.Records {
		if r.Scope == models.InScope {
			snap.InScope++
		}
		if r.Kind == models.KindHost {
			snap.Hosts++
		} else {
			snap.Endpoints++
		}
	}
	p.Snapshots = append(p.Snapshots, snap)
	return Summary{New: snap.New, Total: snap.Total, Runs: runs}, ctx.Err()
}

func fetchOne(ctx context.Context, s sources.Source, domain string, timeout time.Duration, notify func(Event)) outcome {
	run := models.SourceRun{Source: s.Name(), Started: time.Now().UTC()}
	if timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}

	notify(Event{Source: s.Name(), Phase: "checking"})
	if err := s.Available(ctx); err != nil {
		return outcome{run: finish(run, err, 0)}
	}
	notify(Event{Source: s.Name(), Phase: "fetching"})
	found, err := s.Fetch(ctx, domain)
	return outcome{run: finish(run, err, len(found)), findings: found}
}

func finish(run models.SourceRun, err error, n int) models.SourceRun {
	run.Finished = time.Now().UTC()
	run.Findings = n
	run.Status = sources.Classify(err)
	if err != nil {
		run.Error = err.Error()
	}
	return run
}
