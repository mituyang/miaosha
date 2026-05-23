#!/usr/bin/env sh
set -eu

cd "$(dirname "$0")/.."

mkdir -p bin

CGO_ENABLED=0 GOOS=linux GOARCH="${GOARCH:-amd64}" go build -trimpath -ldflags="-s -w" -o bin/api ./cmd/api
CGO_ENABLED=0 GOOS=linux GOARCH="${GOARCH:-amd64}" go build -trimpath -ldflags="-s -w" -o bin/worker ./cmd/worker
