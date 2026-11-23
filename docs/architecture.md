# Architecture

| Package | Responsibility |
|---|---|
| `pkg/models` | Unified schema: Project, Record, Observation, SourceRun, Snapshot |
| `internal/sources` | Provider adapters implementing `Source`, shared HTTP/retry/rate-limit/error classification |
| `internal/scope` | Scope Guard (validation, normalization, verdicts with reasons) |
| `internal/endpoints` | URL normalization and heuristic classification |
| `internal/deduplication` | Identity of a record per dedupe mode |
| `internal/intelligence` | Aggregator: correlation, provenance, scope labelling, confidence |
| `internal/discovery` | Bounded-concurrency runner, timeouts, cancellation, run history, snapshots |
| `internal/storage` | Checksummed atomic project files, search/filter, backup/restore |
| `internal/reporting` | JSON/CSV/TXT/HTML renderers |
| `internal/config`, `logging`, `buildinfo` | Configuration, slog logger, product identity |
| `internal/app` | Service layer shared by CLI and menu |
| `internal/cli`, `internal/tui` | Commands; banner, prompts, progress, interactive menu |

## Rules
- **Confidence**: high = 2+ distinct sources report the record; medium = one source. It measures corroboration, not liveness or risk.
- **Provenance**: each report is an `Observation` (source, discovery time, source-reported time, original string, evidence). Observations are only skipped when identical. Differing original spellings of one normalized URL are all kept.
- **Classification** is derived from the URL path only and stored as `inferred (confidence): rule`.
- **Errors** share one vocabulary: ok, canceled, timeout, rate_limited, unavailable, parse_error, http_error, error.
- **Concurrency**: a semaphore bounds parallel sources; one goroutine owns the aggregator, so no locks guard the data model.
- **Dedupe modes**: `strict` keeps differing query values apart; `params` (default) collapses by parameter names.
