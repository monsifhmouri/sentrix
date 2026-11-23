<h1 align="center">SENTRIX</h1>
<p align="center"><b>ADVANCED ATTACK SURFACE INTELLIGENCE</b></p>
<p align="center">Passive reconnaissance for authorized bug bounty research</p>
<p align="center">
<img alt="Go" src="https://img.shields.io/badge/Go-1.22+-00ADD8?logo=go&logoColor=white">
<img alt="Platforms" src="https://img.shields.io/badge/platform-linux%20%7C%20windows-lightgrey">
<img alt="Arch" src="https://img.shields.io/badge/arch-amd64%20%7C%20arm64-blue">
<img alt="Dependencies" src="https://img.shields.io/badge/dependencies-0-brightgreen">
<img alt="License" src="https://img.shields.io/badge/license-MIT-red">
</p>
<p align="center">Engineered by <b>MONSIF HMOURI</b> (MONSTR MIND) · <b>MONSTR MIND LABS</b><br><a href="https://github.com/monsifhmouri">github.com/monsifhmouri</a></p>

---

## What is SENTRIX

I built SENTRIX because my recon notes kept ending up scattered across tools that each remember a different slice of a target. SENTRIX pulls what public datasets already know about a domain into one project, normalizes it, checks every result against the scope I declared, and keeps the full trail of who reported what.

It is **passive by design**. It talks to public datasets (certificate transparency, web archives, crawl indexes, URL scan databases). It never sends a request to the target, never fuzzes, never tests credentials, never exploits anything. A discovered endpoint is only a discovered endpoint: SENTRIX will not call anything vulnerable.

- Single static binary, no Python, no Docker, no CGO, **zero third-party Go dependencies**
- Linux and Windows, amd64 and arm64
- Works offline for everything except the actual data collection
- Every result keeps its sources, timestamps and evidence

## Contents
[Features](#features) · [Install](#install) · [Quick start](#quick-start) · [Interactive menu](#interactive-menu) · [Command reference](#command-reference) · [Scope rules](#scope-rules) · [Data sources](#data-sources) · [How results are built](#how-results-are-built) · [Output formats](#output-formats) · [Configuration](#configuration) · [Project storage](#project-storage) · [Architecture](#architecture) · [Development](#development) · [Limitations](#known-limitations) · [Responsible use](#responsible-use) · [Roadmap](#roadmap)

## Features
| Area | What it does |
|---|---|
| Sources | crt.sh, Internet Archive CDX, Common Crawl, urlscan.io, AlienVault OTX |
| Scope Guard | roots (with subdomains), exact hosts, URL prefixes, exclusions; exclusions always win; out-of-scope results are kept and labelled, never silently dropped |
| Normalization | scheme and host case, default ports, dot segments, percent-encoding, trailing slash, query ordering, fragments; original URL preserved |
| Classification | OpenAPI/Swagger, GraphQL, JavaScript, sitemap, public config, static, REST-like, documentation, route; always labelled as inferred, with the rule that matched |
| Deduplication | `strict` or `params` mode; differing original spellings of one URL are all kept as observations |
| Confidence | `high` when two or more distinct sources report a record, `medium` for one |
| Projects | incremental scans, per-scan snapshots, search and filter, integrity check, backup and restore |
| Exports | JSON, CSV, TXT, HTML; byte-for-byte reproducible for the same project state |
| Runtime | bounded concurrency, per-source rate limits, retry on 429/503 with `Retry-After`, per-source timeouts, Ctrl-C keeps partial results |
| Terminal | startup banner, interactive menu, live per-source status, plain-ASCII fallback when colour is unavailable |

## Install
You only need Go **1.22+** to build. The resulting binary is standalone.

### Linux
```bash
git clone <repository-url> sentrix && cd sentrix
make build
sudo install bin/sentrix /usr/local/bin/sentrix
sentrix version
```

### Windows (PowerShell)
```powershell
git clone <repository-url> sentrix; cd sentrix
go build -trimpath -o sentrix.exe ./cmd/sentrix
.\sentrix.exe version
```

### All release targets
```bash
make dist                      # or: ./scripts/build.sh 0.1.0
```
Produces `dist/sentrix-<version>-{linux,windows}-{amd64,arm64}[.exe]` and `dist/SHA256SUMS`. Builds are reproducible: set `SOURCE_DATE_EPOCH` and two builds yield identical checksums.
```bash
cd dist && sha256sum -c SHA256SUMS
```

## Quick start
```bash
# 1. review and run a passive scan (you are asked to confirm before any request is sent)
sentrix scan -name acme -domain example.com -scope exclude:dev.example.com

# 2. browse what was found, offline
sentrix show -name acme -scope in -class graphql

# 3. export a report
sentrix export -name acme -format html,csv -out ./reports

# 4. come back later and run it again; only new findings are added
sentrix rescan -name acme
```
Running `sentrix` with no arguments opens the interactive menu instead.

## Interactive menu
```
[1] Create New Reconnaissance Project
[2] Resume Existing Project
[3] Browse Previous Results
[4] Manage Data Sources
[5] Configuration
[6] Update Information
[7] About SENTRIX
[0] Exit
```
| Option | Behaviour |
|---|---|
| 1 | Asks for project name, target domain, extra scope, program name, discovery mode, sources, concurrency, rate limit, output directory and export formats. Every answer is validated and re-asked on error. A review screen is shown and **nothing touches the network until you confirm** |
| 2 | Lists saved projects, shows their configuration, optionally re-runs the scan |
| 3 | Search text, class filter and scope filter over a saved project; fully offline |
| 4 | Shows each source, its access level, whether it is enabled and whether a key is set. A live availability check only runs if you ask for it |
| 5 | Toggle sources, set API keys, set default concurrency; saved to the config file |
| 6 | Version and build information. SENTRIX never checks for updates or phones home |
| 7 | About screen |

## Command reference
Global flags go **before** the command.
```
sentrix [-config PATH] [-no-anim] [-no-color] <command> [flags]
```
| Global flag | Meaning |
|---|---|
| `-config PATH` | configuration file (default `~/.sentrix/config.json`) |
| `-no-anim` | disable the banner animation |
| `-no-color` | force plain ASCII output |

### `scan` create a project and run a passive scan
```bash
sentrix scan -name NAME -domain DOMAIN [flags]
```
| Flag | Default | Meaning |
|---|---|---|
| `-name` | required | project name (stored as a safe slug) |
| `-domain` | required | target root domain |
| `-scope` | none | extra scope entry; repeatable; see [Scope rules](#scope-rules) |
| `-program` | none | program or organization name |
| `-mode` | `passive` | `passive` (up to 5,000 results per source) or `passive-extended` (up to 50,000) |
| `-sources` | all enabled | comma-separated: `crtsh,wayback,commoncrawl,urlscan,otx` |
| `-concurrency` | config (3) | sources processed in parallel, 1 to 32 |
| `-rate` | config (1) | max requests per second per source, 0.05 to 50 |
| `-out` | config | output directory for exports |
| `-format` | config (json,html) | `json,csv,txt,html` |
| `-dedupe` | `params` | `strict` or `params` |
| `-yes` | off | skip the confirmation prompt; you are still asserting that you are authorized |

### `rescan` run an existing project again
```bash
sentrix rescan -name NAME [-yes]
```
Existing records are kept, new observations are added, scope verdicts are recomputed, and a new snapshot is stored.

### `projects` list saved projects
```bash
sentrix projects
```
Damaged project files are listed as unreadable instead of being hidden.

### `show` browse and filter records (offline)
```bash
sentrix show -name NAME [-q TEXT] [-class CLASS] [-source SRC] [-scope in|out] [-kind host|endpoint] [-limit N]
```
| Flag | Meaning |
|---|---|
| `-q` | case-insensitive substring of the normalized URL, original URL or evidence |
| `-class` | e.g. `graphql`, `openapi-docs`, `javascript`, `rest-api`, `sitemap`, `public-config`, `static`, `documentation`, `route`, `hostname` |
| `-source` | only records reported by this source |
| `-scope` | `in` or `out` |
| `-kind` | `host` or `endpoint` |
| `-limit` | max rows printed (default 100) |

### `export` write reports
```bash
sentrix export -name NAME [-format json,csv,txt,html] [-out DIR]
```
Files are written as `<project-slug>.<format>`. Defaults come from the project's saved settings.

### `sources` list sources
```bash
sentrix sources            # offline: name, access level, enabled, key status
sentrix sources -check     # live availability check, one small request per source
```

### `check` verify project integrity
```bash
sentrix check -name NAME
```

### `backup` and `restore`
```bash
sentrix backup -name NAME -to FILE
sentrix restore -from FILE [-overwrite]
```
Backups are verified before they are written, so a damaged project is never backed up silently. Restore refuses to replace an existing project unless `-overwrite` is given.

### Other commands
| Command | Output |
|---|---|
| `config` | active configuration with API keys masked |
| `version` | version, commit, build date, developer |
| `about` | banner and about screen |
| `help` | usage |

### Exit codes
| Code | Meaning |
|---|---|
| 0 | success (including a scan the user cancelled at the review prompt) |
| 1 | runtime or validation error |
| 2 | usage error, unknown command |

## Scope rules
The target domain is always a root. Add more with `-scope` (repeatable; comma lists also work).
| Entry | Meaning |
|---|---|
| `root:corp.example.org` or `corp.example.org` | the domain and all of its subdomains. A leading `*.` is accepted and normalized |
| `host:api.partner.test` | exactly this hostname, no subdomains |
| `url:https://shared.test/program/acme` | this URL prefix on this host, matched on path-segment boundaries (`/acme` and `/acme/x` match, `/acmeevil` does not). Scheme is ignored |
| `exclude:dev.example.com` | this host and everything below it |
| `exclude:https://example.com/admin` | this URL prefix |

Rules of the guard:
- Exclusions are checked first and always win.
- Anything that matches no rule is `out-of-scope`. It is stored with the reason, shown in reports in its own section, and never used for further processing.
- SENTRIX never expands scope on its own to related domains.
- Roots must be real domains, not IP addresses or single labels.
- Every record stores the reason for its verdict (`within root example.com`, `excluded host rule dev.example.com`, and so on).
```bash
sentrix scan -name acme -domain example.com \
  -scope host:api.partner.test \
  -scope url:https://shared.test/program/acme \
  -scope exclude:dev.example.com \
  -scope exclude:https://example.com/admin
```

## Data sources
| Source | Provides | Access | Notes |
|---|---|---|---|
| `crtsh` | hostnames from issued certificates | public | one row per certificate is grouped per hostname; certificate count and date range are kept in the evidence |
| `wayback` | historical URLs from the Internet Archive CDX API | public | one entry per distinct URL; capture timestamp and HTTP status as evidence |
| `commoncrawl` | URLs from Common Crawl | public | newest crawl only; a domain with no captures returns an empty result |
| `urlscan` | URLs from the urlscan.io public search | optional key | works without a key under stricter provider limits |
| `otx` | URLs and passive DNS hostnames from AlienVault OTX | optional key | passive DNS record type, address and first/last seen go into the evidence |

Providers can throttle, change formats, or disappear. SENTRIX does not assume otherwise: each source reports its own status, a failing source never stops the others, and partial results are kept.

| Status | Meaning |
|---|---|
| `ok` | completed |
| `rate_limited` | provider kept answering 429 after the allowed retries |
| `unavailable` | network failure or provider 5xx |
| `timeout` | per-source timeout reached |
| `canceled` | stopped by the user |
| `parse_error` | the provider answered in an unexpected format |
| `http_error` | other HTTP failure (for example 403) |

Retries: up to two on 429/503, honouring `Retry-After` (capped at 30 seconds). Responses larger than 64 MB are rejected.

### API keys
Environment variables win over the config file, so secrets never need to touch disk.
```bash
export SENTRIX_URLSCAN_API_KEY=...
export SENTRIX_OTX_API_KEY=...
```
```powershell
$env:SENTRIX_OTX_API_KEY = "..."
```
Keys are never printed. `sentrix config` masks them.

## How results are built
```
sources -> discovery -> intelligence -> storage -> reporting
```
1. **Collect.** Each source runs in a bounded worker pool with its own rate limiter and timeout.
2. **Normalize.** URLs are canonicalized; the original string is kept as part of the observation.
3. **Label scope.** Every record gets `in-scope` or `out-of-scope` plus the reason.
4. **Classify.** Endpoint class comes from the URL path only and is stored as `inferred (confidence): rule`.
5. **Correlate.** Records are deduplicated under the chosen mode. Observations are never merged away; they are only skipped when they are exactly identical.
6. **Score.** `high` confidence means two or more distinct sources reported it, `medium` means one. This measures corroboration, not that the asset is live and not that it is risky.
7. **Persist.** Results are written atomically and a snapshot (total, new, in-scope, hosts, endpoints) is appended for historical comparison.

URLs also register their host as a host record, once per source.

### Record fields
`id`, `kind` (`host` / `endpoint`), `original_url`, `normalized_url`, `hostname`, `path`, `query_params`, `class`, `class_basis`, `scope`, `scope_reason`, `confidence`, `first_seen`, `observations[]` (`source`, `discovered_at`, `observed_at`, `original`, `evidence`).

### Dedupe modes
| Mode | Behaviour |
|---|---|
| `params` (default) | `/item?id=1` and `/item?id=2` collapse into one record keyed by parameter names. Removes most archive noise |
| `strict` | different query values stay separate records |

### Classes
| Class | Matched by |
|---|---|
| `openapi-docs` | known Swagger / OpenAPI locations (`/swagger.json`, `/openapi.yaml`, `/v3/api-docs`, ...) |
| `graphql` | path ending in or containing `graphql` / `graphiql` |
| `javascript` | `.js`, `.mjs` |
| `sitemap` | file name starting with `sitemap` |
| `public-config` | `robots.txt`, `security.txt`, `/.well-known/*`, `manifest.json`, ... |
| `static` | images, fonts, CSS, media, PDF |
| `rest-api` | path segment `api`, `rest` or `v<N>` (low confidence) |
| `documentation` | path segment `docs`, `documentation`, `developers` (low confidence) |
| `route` | everything else |
| `hostname` | host records |

## Output formats
| Format | Content |
|---|---|
| `json` | tool and developer metadata, project and scope, source runs, snapshots, every record with all observations |
| `csv` | one row per record: id, kind, scope, scope reason, class, class basis, confidence, hostname, path, query params, normalized URL, original URL, sources, observation count, first seen |
| `txt` | summary, source runs, in-scope list, out-of-scope list |
| `html` | project metadata, scope summary, source summary, asset statistics, class table, discovery timeline and snapshots, errors and limitations, in-scope and out-of-scope tables with provenance. Supports light and dark mode. All third-party data is escaped |

Reports never invent findings. They state that the target was not contacted and that classes are inferences. Exports contain no wall-clock timestamp, so exporting the same project twice gives identical files.

## Configuration
Default file: `~/.sentrix/config.json` (created with mode `0600` when saved). A missing file means defaults; omitted fields keep defaults. See `configs/config.example.json`.
```json
{
  "data_dir": "~/.sentrix/projects",
  "output_dir": "~/.sentrix/exports",
  "concurrency": 3,
  "rate_per_second": 1,
  "export_formats": ["json", "html"],
  "log_level": "warn",
  "theme": "auto",
  "startup_animation": true,
  "proxy": "",
  "sources": {
    "crtsh":       { "enabled": true, "rate_per_second": 1,   "timeout_seconds": 120 },
    "wayback":     { "enabled": true, "rate_per_second": 1,   "timeout_seconds": 120 },
    "commoncrawl": { "enabled": true, "rate_per_second": 1,   "timeout_seconds": 120 },
    "urlscan":     { "enabled": true, "rate_per_second": 0.5, "timeout_seconds": 60 },
    "otx":         { "enabled": true, "rate_per_second": 1,   "timeout_seconds": 60 }
  }
}
```
| Key | Meaning |
|---|---|
| `data_dir`, `output_dir` | where projects and exports live (`~` is expanded) |
| `concurrency` | parallel sources, 1 to 32 |
| `rate_per_second` | project-wide ceiling per source; the lower of this and the source's own limit applies |
| `export_formats` | any of `json`, `csv`, `txt`, `html` |
| `log_level` | `debug`, `info`, `warn`, `error`; logs go to stderr |
| `theme` | `auto` or `mono`. `NO_COLOR` and `TERM=dumb` also disable colour |
| `startup_animation` | row-by-row banner reveal. Purely cosmetic; disable with `-no-anim` |
| `proxy` | used only when set explicitly; proxy environment variables are ignored |
| `sources.<name>` | `enabled`, `api_key`, `rate_per_second`, `timeout_seconds` |

## Project storage
```
~/.sentrix/projects/<slug>.sentrix.json     projects
~/.sentrix/exports/<slug>.<format>          exports
~/.sentrix/config.json                      configuration
```
- Writes are atomic (temporary file, fsync, rename), so a crash cannot leave a torn project file.
- Each file carries a SHA-256 checksum verified on every load, backup and restore.
- Access inside one process is serialized. Do not run two scans against the same project at the same time.
- The checksum detects corruption and casual edits. It is not a signature.

## Architecture
```
cmd/sentrix            entry point
internal/cli           commands
internal/tui           banner, prompts, live progress, interactive menu
internal/app           service layer shared by CLI and menu
internal/discovery     bounded-concurrency runner, timeouts, run history, snapshots
internal/intelligence  correlation, provenance, scope labelling, confidence
internal/sources       provider adapters + shared HTTP, retry, rate limit, error classes
internal/scope         Scope Guard
internal/endpoints     URL normalization and classification
internal/deduplication record identity per mode
internal/storage       checksummed atomic project files, search, backup/restore
internal/reporting     JSON, CSV, TXT, HTML
internal/config        configuration and credentials
internal/logging       slog setup
internal/buildinfo     identity and build metadata
pkg/models             unified result schema
```
One goroutine owns the aggregator, so the data model needs no locks; the worker pool is bounded by a semaphore.

### Adding a source
Implement `sources.Source`, register it in `internal/sources/registry.go`, and test it against an `httptest` server.
```go
type Source interface {
    Name() string
    Description() string
    Access() Access
    Available(ctx context.Context) error
    Fetch(ctx context.Context, domain string) ([]Finding, error)
}
```

## Development
```bash
make check     # gofmt, go vet, tests
make race      # tests with the race detector
make build     # bin/sentrix
make dist      # all release targets with SHA256SUMS
```
Tests use `httptest` servers and fixtures. **No test contacts a third-party host.** Coverage includes scope rules, URL normalization, classification, deduplication and provenance, confidence, every source adapter (parsing, retry, rate limit, cancellation, error classes), concurrency bounds, storage integrity, backup/restore, exports and the CLI flow.

## Known limitations
- Standard library only: line-based interactive menu instead of a full-screen TUI, checksummed JSON storage instead of SQLite.
- Internationalized domain names are not converted to punycode.
- Source adapters are tested against mocked responses. Providers change their APIs; if one stops parsing, SENTRIX reports `parse_error` and carries on.
- Result caps per source depend on the mode, and providers may truncate on their side. A missing record is not proof that a resource does not exist.
- Single-process locking only.
- Not implemented yet: technology fingerprinting, DNS record collection, certificate detail beyond hostnames.

## Responsible use
SENTRIX is for assets you are **authorized** to assess. You are responsible for your scope definition, for each bug bounty program's rules, and for the terms of service of the data providers you query. The tool is passive and does not contact targets, but that does not replace authorization. Do not use it to profile systems you have no permission to test.

## Roadmap
- Full-screen TUI and SQLite storage
- IDNA support
- More public sources
- Opt-in, in-scope DNS and TLS enrichment

## Author
**MONSIF HMOURI** (MONSTR MIND) · MONSTR MIND LABS
https://github.com/monsifhmouri

Released under the MIT License. See [LICENSE](LICENSE), [SECURITY.md](SECURITY.md), [CONTRIBUTING.md](CONTRIBUTING.md) and [CHANGELOG.md](CHANGELOG.md).
