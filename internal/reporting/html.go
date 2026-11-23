package reporting

import (
	"bytes"
	"html/template"
	"strings"

	"sentrix/internal/buildinfo"
	"sentrix/pkg/models"
)

type htmlData struct {
	P          *models.Project
	S          Stats
	Info       buildinfoView
	Note       string
	InScope    []models.Record
	OutOfScope []models.Record
	Errors     []models.SourceRun
}

type buildinfoView struct{ Name, Subtitle, Developer, Alias, Org, GitHub, Version string }

var funcs = template.FuncMap{
	"join": strings.Join,
	"date": func(r models.Record) string { return r.FirstSeen.UTC().Format("2006-01-02 15:04") },
	"ts":   func(r models.SourceRun) string { return r.Finished.UTC().Format("2006-01-02 15:04:05") },
	"srcs": func(r models.Record) string { return strings.Join(r.Sources(), ", ") },
}

func renderHTML(p *models.Project) ([]byte, error) {
	d := htmlData{P: p, S: ComputeStats(p), Note: provenanceNote,
		Info: buildinfoView{buildinfo.Name, buildinfo.Subtitle, buildinfo.Developer, buildinfo.Alias, buildinfo.Org, buildinfo.GitHub, buildinfo.Version}}
	for _, r := range p.Records {
		if r.Scope == models.InScope {
			d.InScope = append(d.InScope, r)
		} else {
			d.OutOfScope = append(d.OutOfScope, r)
		}
	}
	for _, r := range p.Runs {
		if r.Status != "ok" || r.Skipped > 0 {
			d.Errors = append(d.Errors, r)
		}
	}
	var b bytes.Buffer
	if err := template.Must(template.New("r").Funcs(funcs).Parse(htmlTemplate)).Execute(&b, d); err != nil {
		return nil, err
	}
	return b.Bytes(), nil
}

const htmlTemplate = `<!DOCTYPE html>
<html lang="en"><head><meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<meta name="generator" content="{{.Info.Name}} {{.Info.Version}} by {{.Info.Developer}}">
<title>{{.Info.Name}} report - {{.P.Name}}</title>
<style>
:root{--bg:#fff;--fg:#17181c;--mut:#5b6070;--card:#f4f5f8;--line:#dcdfe6;--acc:#c8102e}
@media (prefers-color-scheme:dark){:root{--bg:#0e0f12;--fg:#e8e9ee;--mut:#9aa0b0;--card:#181a20;--line:#2b2e38;--acc:#ff4d63}}
*{box-sizing:border-box}body{margin:0;background:var(--bg);color:var(--fg);font:15px/1.5 system-ui,-apple-system,Segoe UI,sans-serif}
header{background:#0e0f12;color:#fff;padding:28px 6vw;border-bottom:4px solid var(--acc)}
header h1{margin:0;letter-spacing:.2em;font-size:28px}header p{margin:4px 0 0;color:#b8bccb}
main{padding:24px 6vw;max-width:1280px;margin:auto}h2{margin:32px 0 10px;border-bottom:1px solid var(--line);padding-bottom:6px}
.grid{display:grid;grid-template-columns:repeat(auto-fit,minmax(150px,1fr));gap:12px}
.card{background:var(--card);border:1px solid var(--line);border-radius:8px;padding:12px 14px}.card b{display:block;font-size:24px}.card span{color:var(--mut)}
.note{border-left:4px solid var(--acc);background:var(--card);padding:10px 14px;margin:16px 0}
.scroll{overflow-x:auto}table{border-collapse:collapse;width:100%}th,td{text-align:left;padding:6px 10px;border-bottom:1px solid var(--line);vertical-align:top}
th{background:var(--card);position:sticky;top:0}td.u{word-break:break-all;font-family:ui-monospace,Consolas,monospace;font-size:13px}
.tag{display:inline-block;border:1px solid var(--line);border-radius:4px;padding:0 6px;font-size:12px;color:var(--mut)}
footer{padding:24px 6vw;color:var(--mut);border-top:1px solid var(--line)}a{color:var(--acc)}
</style></head><body>
<header><h1>{{.Info.Name}}</h1><p>{{.Info.Subtitle}}</p>
<p>Engineered by {{.Info.Developer}} ({{.Info.Alias}}) &middot; {{.Info.Org}} &middot; <a href="{{.Info.GitHub}}">{{.Info.GitHub}}</a></p></header>
<main>
<div class="note">{{.Note}}</div>

<h2>Project</h2>
<div class="scroll"><table>
<tr><th>Name</th><td>{{.P.Name}}</td></tr><tr><th>Target</th><td>{{.P.Domain}}</td></tr>
{{if .P.Program}}<tr><th>Program</th><td>{{.P.Program}}</td></tr>{{end}}
<tr><th>Mode</th><td>{{.P.Mode}} (dedupe: {{.P.DedupeMode}})</td></tr>
<tr><th>Created / updated</th><td>{{.P.CreatedAt.UTC.Format "2006-01-02 15:04"}} / {{.P.UpdatedAt.UTC.Format "2006-01-02 15:04"}} UTC</td></tr>
</table></div>

<h2>Scope summary</h2>
<div class="scroll"><table>
<tr><th>Roots (incl. subdomains)</th><td>{{join .P.Scope.Roots ", "}}</td></tr>
{{if .P.Scope.Hosts}}<tr><th>Exact hosts</th><td>{{join .P.Scope.Hosts ", "}}</td></tr>{{end}}
{{if .P.Scope.URLPrefixes}}<tr><th>URL prefixes</th><td>{{join .P.Scope.URLPrefixes ", "}}</td></tr>{{end}}
{{if .P.Scope.Exclude}}<tr><th>Exclusions</th><td>{{join .P.Scope.Exclude ", "}}</td></tr>{{end}}
</table></div>

<h2>Asset statistics</h2>
<div class="grid">
<div class="card"><b>{{.S.Hosts}}</b><span>hosts</span></div>
<div class="card"><b>{{.S.Endpoints}}</b><span>endpoints</span></div>
<div class="card"><b>{{.S.InScope}}</b><span>in scope</span></div>
<div class="card"><b>{{.S.OutScope}}</b><span>out of scope</span></div>
<div class="card"><b>{{.S.MultiSource}}</b><span>seen by 2+ sources</span></div>
<div class="card"><b>{{.S.WithVariants}}</b><span>with differing original spellings</span></div>
</div>

<h2>Source summary</h2>
<div class="scroll"><table><tr><th>Source</th><th>Status</th><th>Findings</th><th>Skipped</th><th>Finished (UTC)</th></tr>
{{range .P.Runs}}<tr><td>{{.Source}}</td><td>{{.Status}}</td><td>{{.Findings}}</td><td>{{.Skipped}}</td><td>{{ts .}}</td></tr>{{else}}<tr><td colspan="5">No scans have been run.</td></tr>{{end}}
</table></div>

<h2>Endpoint classifications <span class="tag">inferred from URL patterns, not verified</span></h2>
<div class="scroll"><table><tr><th>Class</th><th>Records</th></tr>{{range .S.ByClass}}<tr><td>{{.Label}}</td><td>{{.N}}</td></tr>{{end}}</table></div>

<h2>Discovery timeline</h2>
<div class="scroll"><table><tr><th>Date first seen</th><th>New records</th></tr>{{range .S.Timeline}}<tr><td>{{.Label}}</td><td>{{.N}}</td></tr>{{end}}</table></div>
{{if .P.Snapshots}}<div class="scroll"><table><tr><th>Scan snapshot (UTC)</th><th>Total</th><th>New</th><th>In scope</th></tr>
{{range .P.Snapshots}}<tr><td>{{.At.UTC.Format "2006-01-02 15:04"}}</td><td>{{.Total}}</td><td>{{.New}}</td><td>{{.InScope}}</td></tr>{{end}}</table></div>{{end}}

<h2>Errors and limitations</h2>
{{if .Errors}}<div class="scroll"><table><tr><th>Source</th><th>Status</th><th>Detail</th></tr>
{{range .Errors}}<tr><td>{{.Source}}</td><td>{{.Status}}</td><td>{{.Error}}{{if .Skipped}} {{.Skipped}} finding(s) could not be parsed.{{end}}</td></tr>{{end}}</table></div>{{else}}<p>No source errors were recorded.</p>{{end}}
<p>Sources return only what their datasets contain and may truncate results; absence of a record is not evidence of absence. No verified observations are included: SENTRIX did not contact the target.</p>

<h2>In-scope records ({{len .InScope}})</h2>
{{template "records" .InScope}}
<h2>Out-of-scope records ({{len .OutOfScope}}) <span class="tag">excluded from further processing</span></h2>
{{template "records" .OutOfScope}}
</main>
<footer>{{.Info.Name}} {{.Info.Version}} &middot; {{.Info.Developer}} / {{.Info.Org}} &middot; <a href="{{.Info.GitHub}}">{{.Info.GitHub}}</a></footer>
</body></html>
{{define "records"}}<div class="scroll"><table>
<tr><th>Kind</th><th>Resource</th><th>Class (inferred)</th><th>Confidence</th><th>Provenance</th><th>First seen</th></tr>
{{range .}}<tr><td>{{.Kind}}</td><td class="u">{{.NormalizedURL}}{{if gt (len .OriginalVariants) 1}}<br><span class="tag">{{len .OriginalVariants}} original spellings</span>{{end}}</td>
<td>{{.Class}}<br><span class="tag" title="{{.ClassBasis}}">{{.ClassBasis}}</span></td><td>{{.Confidence}}</td>
<td>{{srcs .}} <span class="tag">{{len .Observations}} obs.</span><br><span class="tag">{{.ScopeReason}}</span></td><td>{{date .}}</td></tr>
{{else}}<tr><td colspan="6">None.</td></tr>{{end}}
</table></div>{{end}}
`
