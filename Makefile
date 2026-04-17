.PHONY: build install test test-unit test-integration test-e2e lint fmt vet security check tools clean

# Install dev tools at pinned versions (run once after cloning)
tools:
	@bash scripts/install-tools.sh

VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")

build:
	go build -ldflags "-s -w -X main.version=$(VERSION)" -o bin/go-apply ./cmd/go-apply/

install: clean build
	install -m 0755 bin/go-apply "$${INSTALL_DIR:-$$HOME/.local/bin}/go-apply"

test: test-unit test-integration

test-unit:
	go test -race ./internal/...

test-integration:
	go test -race -tags integration ./...

test-e2e:
	go test -race -tags e2e ./tests/e2e/...

lint:
	golangci-lint run ./...

fmt:
	goimports -w .

vet:
	go vet ./...

security:
	govulncheck ./...
	gosec ./...

# check mirrors what CI runs — use before pushing (requires: make tools)
check: vet lint security test-unit

clean:
	rm -rf bin/
