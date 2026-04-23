.PHONY: build install test test-unit test-integration test-e2e lint fmt vet security check tools clean sync-skill

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

# sync-skill — vendor the resume-tailor SKILL.md into the embedded skills directory.
# RESUME_TAILOR_SKILL_SRC defaults to the maintainer's grimoire path.
# For other contributors, the vendored internal/mcpserver/skills/resume-tailor.md IS the source of truth.
RESUME_TAILOR_SKILL_SRC ?= $(HOME)/workplace/the-scriptorium/grimoire/skills/resume-tailor/SKILL.md
SKILL_DST      := internal/mcpserver/skills/resume-tailor.md
SKILL_HASH_DST := internal/mcpserver/skills/resume-tailor.md.sha256

sync-skill:
	@test -f "$(RESUME_TAILOR_SKILL_SRC)" || (echo "SKILL.md not found at $(RESUME_TAILOR_SKILL_SRC)"; exit 1)
	@cp "$(RESUME_TAILOR_SKILL_SRC)" "$(SKILL_DST).tmp"
	@test "$$(wc -c <"$(SKILL_DST).tmp")" -gt 2000 || (echo "source file suspiciously small; aborting"; rm -f "$(SKILL_DST).tmp"; exit 1)
	@mv "$(SKILL_DST).tmp" "$(SKILL_DST)"
	@shasum -a 256 "$(SKILL_DST)" | awk '{print $$1}' > "$(SKILL_HASH_DST)"
	@echo "Synced $(SKILL_DST) and refreshed $(SKILL_HASH_DST)."
	@echo "Commit BOTH files together. Run 'go test ./internal/mcpserver/...' to verify."
