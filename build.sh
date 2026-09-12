#!/usr/bin/env sh
# Compila o Bifrost para Windows, Linux e macOS (cross-compile, sem CGO).
# Uso: ./build.sh [versao]
set -e
cd "$(dirname "$0")"
VERSION="${1:-1.0.0}"
mkdir -p dist

go run ./tools/genres "$VERSION"

echo "Windows amd64..."
GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -trimpath \
  -ldflags "-H windowsgui -s -w -X main.version=$VERSION" -o dist/bifrost-windows-amd64.exe ./cmd/bifrost

echo "Linux amd64..."
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -trimpath \
  -ldflags "-s -w -X main.version=$VERSION" -o dist/bifrost-linux-amd64 ./cmd/bifrost

echo "Linux arm64..."
GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -trimpath \
  -ldflags "-s -w -X main.version=$VERSION" -o dist/bifrost-linux-arm64 ./cmd/bifrost

echo "macOS amd64 (Intel)..."
GOOS=darwin GOARCH=amd64 CGO_ENABLED=0 go build -trimpath \
  -ldflags "-s -w -X main.version=$VERSION" -o dist/bifrost-darwin-amd64 ./cmd/bifrost

echo "macOS arm64 (Apple Silicon)..."
GOOS=darwin GOARCH=arm64 CGO_ENABLED=0 go build -trimpath \
  -ldflags "-s -w -X main.version=$VERSION" -o dist/bifrost-darwin-arm64 ./cmd/bifrost

echo "Pronto: binários gerados em dist/"
