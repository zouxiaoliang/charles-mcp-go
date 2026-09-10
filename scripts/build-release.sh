#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/.."
mkdir -p dist
for target_os in darwin linux windows; do
  for target_arch in amd64 arm64; do
    artifact="dist/charles-mcp-${target_os}-${target_arch}"
    if [[ "$target_os" == windows ]]; then artifact+=".exe"; fi
    CGO_ENABLED=0 GOOS="$target_os" GOARCH="$target_arch" go build -trimpath -ldflags='-s -w' -o "$artifact" ./cmd/charles-mcp
  done
done
