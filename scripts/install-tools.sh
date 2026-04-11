#!/usr/bin/env bash
# install-tools.sh — install dev tools at pinned versions.
# Run once after cloning: make tools
# Versions here must match ci.yml.
set -euo pipefail

GOLANGCI_LINT_VERSION="v1.64.8"
GOSEC_VERSION="latest"
GOVULNCHECK_VERSION="latest"
GOIMPORTS_VERSION="latest"

echo "Installing golangci-lint ${GOLANGCI_LINT_VERSION}..."
go install "github.com/golangci/golangci-lint/cmd/golangci-lint@${GOLANGCI_LINT_VERSION}"

echo "Installing gosec ${GOSEC_VERSION}..."
go install "github.com/securego/gosec/v2/cmd/gosec@${GOSEC_VERSION}"

echo "Installing govulncheck ${GOVULNCHECK_VERSION}..."
go install "golang.org/x/vuln/cmd/govulncheck@${GOVULNCHECK_VERSION}"

echo "Installing goimports ${GOIMPORTS_VERSION}..."
go install "golang.org/x/tools/cmd/goimports@${GOIMPORTS_VERSION}"

echo "All tools installed."
