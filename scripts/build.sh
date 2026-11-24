#!/usr/bin/env bash
# Cross-compiles SENTRIX for all supported targets and writes SHA-256 sums.
set -euo pipefail
cd "$(dirname "$0")/.."

VERSION="${1:-0.1.0}"
COMMIT="$(git rev-parse --short HEAD 2>/dev/null || echo unknown)"
EPOCH="${SOURCE_DATE_EPOCH:-0}"
DATE="$(date -u -d "@${EPOCH}" +%Y-%m-%dT%H:%M:%SZ 2>/dev/null || echo unknown)"
PKG="sentrix/internal/buildinfo"
LDFLAGS="-s -w -buildid= -X ${PKG}.Version=${VERSION} -X ${PKG}.Commit=${COMMIT} -X ${PKG}.Date=${DATE}"

rm -rf dist && mkdir -p dist
for target in linux/amd64 linux/arm64 windows/amd64 windows/arm64; do
  os="${target%/*}"; arch="${target#*/}"; ext=""
  [ "$os" = windows ] && ext=".exe"
  out="dist/sentrix-${VERSION}-${os}-${arch}${ext}"
  echo "building ${out}"
  CGO_ENABLED=0 GOOS="$os" GOARCH="$arch" go build -trimpath -ldflags "$LDFLAGS" -o "$out" ./cmd/sentrix
done
(cd dist && sha256sum sentrix-* > SHA256SUMS)
echo "done; checksums in dist/SHA256SUMS"
