#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/.."
version="${VERSION:-$(git describe --tags --exact-match 2>/dev/null || printf 'dev')}"
if [[ ! "$version" =~ ^[[:alnum:]][[:alnum:].+_-]*$ ]]; then
  printf 'Invalid build version: %s\n' "$version" >&2
  exit 1
fi
mkdir -p dist
artifacts=()
for target_os in darwin linux windows; do
  for target_arch in amd64 arm64; do
    artifact="dist/charles-mcp-${target_os}-${target_arch}"
    if [[ "$target_os" == windows ]]; then artifact+=".exe"; fi
    CGO_ENABLED=0 GOOS="$target_os" GOARCH="$target_arch" go build -trimpath -ldflags="-s -w -X main.version=$version" -o "$artifact" ./cmd/charles-mcp
    artifacts+=("${artifact#dist/}")
  done
done
cd dist
if command -v sha256sum >/dev/null 2>&1; then
  sha256sum "${artifacts[@]}" > SHA256SUMS
else
  shasum -a 256 "${artifacts[@]}" > SHA256SUMS
fi
