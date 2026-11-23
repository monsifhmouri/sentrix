VERSION ?= 0.1.0
COMMIT  ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo unknown)
DATE    ?= $(shell date -u -d "@$${SOURCE_DATE_EPOCH:-$$(date +%s)}" +%Y-%m-%dT%H:%M:%SZ 2>/dev/null || echo unknown)
PKG     := sentrix/internal/buildinfo
LDFLAGS := -s -w -buildid= -X $(PKG).Version=$(VERSION) -X $(PKG).Commit=$(COMMIT) -X $(PKG).Date=$(DATE)
TARGETS := linux/amd64 linux/arm64 windows/amd64 windows/arm64

.PHONY: build test race vet fmt check dist clean

build:
	CGO_ENABLED=0 go build -trimpath -ldflags "$(LDFLAGS)" -o bin/sentrix ./cmd/sentrix

test:
	go test -count=1 ./...

race:
	go test -race -count=1 ./...

vet:
	go vet ./...

fmt:
	@test -z "$$(gofmt -l .)" || (echo "gofmt needed:"; gofmt -l .; exit 1)

check: fmt vet test

# Cross-compile every supported target into dist/ with checksums.
# Set SOURCE_DATE_EPOCH to make the embedded build date reproducible.
dist:
	./scripts/build.sh "$(VERSION)"

clean:
	rm -rf bin dist
