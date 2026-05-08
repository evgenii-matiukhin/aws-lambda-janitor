#!/usr/bin/env bash
set -euo pipefail

echo "Installing dependencies..."
go mod download
go mod tidy

echo "Running tests..."
go test -v ./cmd/janitor/...

echo "Building CDK app..."
go build ./...

echo "Build complete."
