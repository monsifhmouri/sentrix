// Package reporting renders a project to JSON, CSV, TXT and HTML. Output is
// a pure function of the project state (no wall-clock timestamps), so
// exporting the same project twice yields identical files.
package reporting

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"sentrix/internal/buildinfo"
	"sentrix/internal/storage"
	"sentrix/pkg/models"
)

// Stats are derived counts shown in reports.
type Stats struct {
	Hosts, Endpoints  int
	InScope, OutScope int
	ByClass           []Count
	BySource          []Count
	Timeline          []Count // records first seen per day
	MultiSource       int     // records corroborated by 2+ sources
	WithVariants      int     // records with differing original spellings
}

type Count struct {
	Label string
	N     int
}

func ComputeStats(p *models.Project) Stats {
	var s Stats
	class, source, day := map[string]int{}, map[string]int{}, map[string]int{}
	for _, r := range p.Records {
		if r.Kind == models.KindHost {
			s.Hosts++
		} else {
			s.Endpoints++
		}
		if r.Scope == models.InScope {
			s.InScope++
		} else {
			s.OutScope++
		}
		class[r.Class]++
		for _, src := range r.Sources() {
			source[src]++
		}
		day[r.FirstSeen.UTC().Format("2006-01-02")]++
		if len(r.Sources()) > 1 {
			s.MultiSource++
		}
		if len(r.OriginalVariants()) > 1 {
			s.WithVariants++
		}
	}
	s.ByClass, s.BySource, s.Timeline = sorted(class, true), sorted(source, true), sorted(day, false)
	return s
}

func sorted(m map[string]int, byCount bool) []Count {
	out := make([]Count, 0, len(m))
	for k, v := range m {
		out = append(out, Count{k, v})
	}
	sort.Slice(out, func(i, j int) bool {
		if byCount && out[i].N != out[j].N {
			return out[i].N > out[j].N
		}
		return out[i].Label < out[j].Label
	})
	return out
}

// Write renders one format into dir and returns the file path.
func Write(p *models.Project, format, dir string) (string, error) {
	var (
		data []byte
		err  error
	)
	switch format {
	case "json":
		data, err = renderJSON(p)
	case "csv":
		data, err = renderCSV(p)
	case "txt":
		data = renderTXT(p)
	case "html":
		data, err = renderHTML(p)
	default:
		return "", fmt.Errorf("unknown export format %q", format)
	}
	if err != nil {
		return "", err
	}
	slug, err := storage.Slug(p.Name)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	path := filepath.Join(dir, slug+"."+format)
	return path, os.WriteFile(path, data, 0o644)
}

type jsonExport struct {
	Tool      string             `json:"tool"`
	Developer string             `json:"developer"`
	GitHub    string             `json:"github"`
	Version   string             `json:"version"`
	Note      string             `json:"note"`
	Project   projectMeta        `json:"project"`
	Runs      []models.SourceRun `json:"source_runs"`
	Snapshots []models.Snapshot  `json:"snapshots"`
	Records   []models.Record    `json:"records"`
}

type projectMeta struct {
	Name, Domain, Program, Mode string
	Scope                       models.ScopeRules
	Sources                     []string
	DedupeMode                  string
	CreatedAt, UpdatedAt        string
}

const provenanceNote = "Records are third-party public-data observations; SENTRIX did not contact the target. " +
	"Classes are heuristic inferences from the URL, not verified facts. Existence of a record does not imply a vulnerability."

func meta(p *models.Project) projectMeta {
	return projectMeta{p.Name, p.Domain, p.Program, p.Mode, p.Scope, p.Sources, p.DedupeMode,
		p.CreatedAt.UTC().Format("2006-01-02T15:04:05Z"), p.UpdatedAt.UTC().Format("2006-01-02T15:04:05Z")}
}

func renderJSON(p *models.Project) ([]byte, error) {
	out := jsonExport{
		Tool: buildinfo.Name + " - " + buildinfo.Subtitle, Developer: buildinfo.Developer + " / " + buildinfo.Org,
		GitHub: buildinfo.GitHub, Version: buildinfo.Version, Note: provenanceNote,
		Project: meta(p), Runs: p.Runs, Snapshots: p.Snapshots, Records: p.Records,
	}
	return json.MarshalIndent(out, "", "  ")
}

func renderCSV(p *models.Project) ([]byte, error) {
	var b bytes.Buffer
	w := csv.NewWriter(&b)
	_ = w.Write([]string{"id", "kind", "scope", "scope_reason", "class", "class_basis", "confidence", "hostname", "path",
		"query_params", "normalized_url", "original_url", "sources", "observations", "first_seen"})
	for _, r := range p.Records {
		_ = w.Write([]string{r.ID, string(r.Kind), string(r.Scope), r.ScopeReason, r.Class, r.ClassBasis, string(r.Confidence),
			r.Hostname, r.Path, strings.Join(r.QueryParams, ";"), r.NormalizedURL, r.OriginalURL,
			strings.Join(r.Sources(), ";"), fmt.Sprint(len(r.Observations)), r.FirstSeen.UTC().Format("2006-01-02T15:04:05Z")})
	}
	w.Flush()
	return b.Bytes(), w.Error()
}

func renderTXT(p *models.Project) []byte {
	var b strings.Builder
	st := ComputeStats(p)
	fmt.Fprintf(&b, "%s - %s\nEngineered by %s / %s\n%s\n\n", buildinfo.Name, buildinfo.Subtitle, buildinfo.Developer, buildinfo.Org, buildinfo.GitHub)
	fmt.Fprintf(&b, "Project: %s\nTarget:  %s\nMode:    %s\n", p.Name, p.Domain, p.Mode)
	if p.Program != "" {
		fmt.Fprintf(&b, "Program: %s\n", p.Program)
	}
	fmt.Fprintf(&b, "Records: %d (%d hosts, %d endpoints); %d in scope, %d out of scope\n\n", len(p.Records), st.Hosts, st.Endpoints, st.InScope, st.OutScope)
	fmt.Fprintf(&b, "NOTE: %s\n\n", provenanceNote)

	b.WriteString("SOURCE RUNS\n")
	for _, r := range p.Runs {
		fmt.Fprintf(&b, "  %-12s %-13s findings=%d skipped=%d %s\n", r.Source, r.Status, r.Findings, r.Skipped, r.Error)
	}
	for _, scope := range []models.ScopeStatus{models.InScope, models.OutOfScope} {
		fmt.Fprintf(&b, "\n%s\n", strings.ToUpper(string(scope)))
		for _, r := range p.Records {
			if r.Scope == scope {
				fmt.Fprintf(&b, "  [%s/%s] %s  (%s)\n", r.Kind, r.Class, r.NormalizedURL, strings.Join(r.Sources(), ","))
			}
		}
	}
	return []byte(b.String())
}
