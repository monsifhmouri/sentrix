// Package intelligence correlates raw source findings into unified Records.
//
// Confidence rules (documented in docs/architecture.md):
//   - high:   the record was reported by two or more distinct sources
//   - medium: the record was reported by exactly one source
//
// Confidence says how well-corroborated the observation is. It is not a claim
// that the resource is live, and says nothing about vulnerability.
package intelligence

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"time"

	"sentrix/internal/deduplication"
	"sentrix/internal/endpoints"
	"sentrix/internal/scope"
	"sentrix/internal/sources"
	"sentrix/pkg/models"
)

// Aggregator is not safe for concurrent use; the discovery pipeline feeds it
// from a single goroutine.
type Aggregator struct {
	guard *scope.Guard
	mode  deduplication.Mode
	byKey map[string]*models.Record
	added int
}

// New seeds the aggregator with existing records so scans are incremental.
// Scope verdicts are recomputed, so edited scope rules apply retroactively.
func New(guard *scope.Guard, mode deduplication.Mode, existing []models.Record) *Aggregator {
	a := &Aggregator{guard: guard, mode: mode, byKey: map[string]*models.Record{}}
	for i := range existing {
		r := existing[i]
		a.applyScope(&r)
		key := deduplication.Key(r.Kind, r.NormalizedURL, mode)
		if cur, ok := a.byKey[key]; ok { // dedupe mode changed since last run
			cur.Observations = append(cur.Observations, r.Observations...)
			setConfidence(cur)
			continue
		}
		a.byKey[key] = &r
	}
	return a
}

// NewRecords reports how many records were created since construction.
func (a *Aggregator) NewRecords() int { return a.added }

func (a *Aggregator) applyScope(r *models.Record) {
	var v scope.Verdict
	if r.Kind == models.KindHost {
		v = a.guard.CheckHost(r.Hostname)
	} else {
		v = a.guard.CheckURL(r.NormalizedURL)
	}
	r.Scope, r.ScopeReason = models.OutOfScope, v.Reason
	if v.InScope {
		r.Scope = models.InScope
	}
}

// Add folds one finding into the record set. It returns an error when the
// finding cannot be parsed; callers count these rather than dropping silently.
func (a *Aggregator) Add(source string, f sources.Finding, now time.Time) error {
	obs := models.Observation{Source: source, DiscoveredAt: now, OriginalURL: f.Raw, Evidence: f.Evidence}
	if !f.ObservedAt.IsZero() {
		t := f.ObservedAt
		obs.ObservedAt = &t
	}

	if f.Kind == models.KindHost {
		host, wild, err := scope.NormalizeHost(f.Raw)
		if err != nil {
			return err
		}
		if wild {
			obs.Evidence += "; reported as wildcard name *." + host
		}
		a.addHost(host, obs, now)
		return nil
	}

	p, err := endpoints.Normalize(f.Raw)
	if err != nil {
		return err
	}
	key := deduplication.Key(models.KindEndpoint, p.Normalized, a.mode)
	rec := a.byKey[key]
	if rec == nil {
		c := endpoints.Classify(p.Path)
		rec = &models.Record{
			Kind: models.KindEndpoint, OriginalURL: f.Raw, NormalizedURL: p.Normalized,
			Hostname: p.Host, Path: p.Path, QueryParams: p.Params,
			Class: c.Class, ClassBasis: fmt.Sprintf("inferred (%s confidence): %s", c.Confidence, c.Rule),
			FirstSeen: now,
		}
		a.create(key, rec)
	}
	a.attach(rec, obs)

	// Every URL also evidences its host. The evidence text is constant so
	// attach collapses it to one observation per source and host.
	hostObs := models.Observation{Source: source, DiscoveredAt: now, OriginalURL: p.Host,
		Evidence: "host appears in URLs reported by this source"}
	a.addHost(p.Host, hostObs, now)
	return nil
}

func (a *Aggregator) addHost(host string, obs models.Observation, now time.Time) {
	key := deduplication.Key(models.KindHost, host, a.mode)
	rec := a.byKey[key]
	if rec == nil {
		rec = &models.Record{
			Kind: models.KindHost, OriginalURL: obs.OriginalURL, NormalizedURL: host, Hostname: host,
			Class: "hostname", ClassBasis: "observed hostname", FirstSeen: now,
		}
		a.create(key, rec)
	}
	a.attach(rec, obs)
}

func (a *Aggregator) create(key string, rec *models.Record) {
	sum := sha256.Sum256([]byte(key))
	rec.ID = hex.EncodeToString(sum[:6])
	a.applyScope(rec)
	a.byKey[key] = rec
	a.added++
}

func (a *Aggregator) attach(rec *models.Record, obs models.Observation) {
	for _, o := range rec.Observations {
		if o.Source == obs.Source && o.OriginalURL == obs.OriginalURL && o.Evidence == obs.Evidence {
			return
		}
	}
	rec.Observations = append(rec.Observations, obs)
	setConfidence(rec)
}

func setConfidence(r *models.Record) {
	if len(r.Sources()) >= 2 {
		r.Confidence = models.ConfidenceHigh
	} else {
		r.Confidence = models.ConfidenceMedium
	}
}

// Records returns a deterministic, sorted copy of the current state.
func (a *Aggregator) Records() []models.Record {
	out := make([]models.Record, 0, len(a.byKey))
	for _, r := range a.byKey {
		out = append(out, *r)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Kind != out[j].Kind {
			return out[i].Kind < out[j].Kind
		}
		return out[i].NormalizedURL < out[j].NormalizedURL
	})
	return out
}
