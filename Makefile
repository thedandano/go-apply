.PHONY: build test test-unit test-integration test-e2e lint fmt vet security clean

build:
	go build -o bin/go-apply ./cmd/go-apply/

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
	gosec ./...

clean:
	rm -rf bin/
