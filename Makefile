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

# Sync the vendored /resume-tailor skill body from the grimoire vault.
# Overridable for CI and contributors whose vault lives elsewhere. Atomic
# write via tmpfile + rename to avoid a half-written file.
RESUME_TAILOR_SKILL_SRC ?= $(HOME)/workplace/the-scriptorium/grimoire/skills/resume-tailor/SKILL.md

.PHONY: sync-tailor-prompt
sync-tailor-prompt:
	@test -f "$(RESUME_TAILOR_SKILL_SRC)" || { echo "source not found: $(RESUME_TAILOR_SKILL_SRC)"; exit 1; }
	cp "$(RESUME_TAILOR_SKILL_SRC)" internal/mcpserver/skills/resume-tailor.md.tmp
	mv internal/mcpserver/skills/resume-tailor.md.tmp internal/mcpserver/skills/resume-tailor.md
	@echo "synced resume-tailor.md from $(RESUME_TAILOR_SKILL_SRC)"
