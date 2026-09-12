#!/usr/bin/env sh
# Compila o bifrost.exe a partir de Linux/macOS (cross-compile, sem CGO).
set -e
cd "$(dirname "$0")"
VERSION="${1:-1.0.0}"
go run ./tools/genres "$VERSION"
GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -trimpath \
  -ldflags "-H windowsgui -s -w -X main.version=$VERSION" -o dist/bifrost.exe ./cmd/bifrost
echo "Pronto: dist/bifrost.exe"
