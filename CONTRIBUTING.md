# Contributing

1. `make check` must pass (gofmt, go vet, tests). Run `make race` for concurrency changes.
2. New source adapters implement `sources.Source`, register in `internal/sources/registry.go`,
   and ship with tests against `httptest` servers. Tests must never contact third-party hosts.
3. Keep the standard library unless a dependency is clearly justified in `docs/dependencies.md`.
4. Features that send requests to a target (not a public dataset) are out of scope for this project.
