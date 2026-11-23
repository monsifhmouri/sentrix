package tui

import (
	"fmt"
	"io"
	"sync/atomic"

	"sentrix/internal/discovery"
	"sentrix/pkg/models"
)

// Progress prints live per-source status. Every line reflects a real event
// from the pipeline; nothing is simulated.
type Progress struct {
	w     io.Writer
	t     Theme
	total atomic.Int64
}

func NewProgress(w io.Writer, t Theme) *Progress { return &Progress{w: w, t: t} }

// Notify is the discovery.Options.Notify callback (calls are already serialized).
func (p *Progress) Notify(e discovery.Event) {
	switch e.Phase {
	case "checking":
		fmt.Fprintf(p.w, "  %s %-12s checking availability\n", p.t.Mark("run"), e.Source)
	case "fetching":
		fmt.Fprintf(p.w, "  %s %-12s fetching\n", p.t.Mark("run"), e.Source)
	case "done":
		mark := p.t.Mark("fail")
		if e.Status == "ok" {
			mark = p.t.Mark("ok")
			p.total.Add(int64(e.Count))
		} else if e.Count > 0 {
			mark = p.t.Mark("skip")
			p.total.Add(int64(e.Count))
		}
		fmt.Fprintf(p.w, "  %s %-12s %-13s %d finding(s)  %s\n", mark, e.Source, e.Status, e.Count,
			p.t.Dim(fmt.Sprintf("(running total %d)", p.total.Load())))
	}
}

// PrintSummary prints the post-scan result block, including failures.
func PrintSummary(w io.Writer, t Theme, sum discovery.Summary) {
	fmt.Fprint(w, t.Header("Scan summary"))
	fmt.Fprintf(w, "  records: %d total, %d new this scan\n", sum.Total, sum.New)
	for _, r := range sum.Runs {
		if r.Status != "ok" || r.Skipped > 0 {
			fmt.Fprintf(w, "  %s %s: %s", t.Warn("warning"), r.Source, r.Status)
			if r.Skipped > 0 {
				fmt.Fprintf(w, " (%d finding(s) could not be parsed)", r.Skipped)
			}
			if r.Error != "" {
				fmt.Fprintf(w, "\n      %s", t.Dim(r.Error))
			}
			fmt.Fprintln(w)
		}
	}
}

// PrintRecords lists up to limit records in a compact, width-independent form.
func PrintRecords(w io.Writer, t Theme, recs []models.Record, limit int) {
	for i, r := range recs {
		if i >= limit {
			fmt.Fprintf(w, "  %s\n", t.Dim(fmt.Sprintf("... %d more; refine the filter or export for the full list", len(recs)-limit)))
			return
		}
		scopeTag := t.OK("in")
		if r.Scope != models.InScope {
			scopeTag = t.Warn("out")
		}
		fmt.Fprintf(w, "  [%s] %-8s %-12s %s  %s\n", scopeTag, r.Kind, r.Class, r.NormalizedURL, t.Dim(fmt.Sprint(r.Sources())))
	}
}
