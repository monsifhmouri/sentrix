# Dependencies

SENTRIX 0.1.0 has **no third-party dependencies**; `go.mod` lists none.

The directive preferred Cobra, Bubble Tea, Lip Gloss and SQLite. The build environment used for
this release could not reach the Go module proxy, so those were replaced by the standard library:
`flag` (commands), ANSI escapes (styling), line-based prompts (menu), checksummed JSON (storage),
`log/slog` (logging), `html/template` (HTML reports). Re-introducing them is a roadmap item and
requires justification here per dependency.
