VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
COMMIT  ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo "none")
DATE    ?= $(shell date -u +"%Y-%m-%dT%H:%M:%SZ")
BINARY  := bin/aibox

LDFLAGS := -X main.version=$(VERSION) \
           -X main.commit=$(COMMIT) \
           -X main.date=$(DATE)

.PHONY: build test test-unit test-integration test-security benchmark lint clean

build:
	CGO_ENABLED=0 go build -ldflags '$(LDFLAGS)' -o $(BINARY) .

test: test-unit

test-unit:
	go test ./internal/...

test-integration: build
	go test -tags=integration -timeout 300s ./tests/integration/

test-security: build
	go test -tags=security -timeout 120s ./tests/security/

test-all: test-unit test-integration test-security

benchmark: build
	./tests/benchmark/run.sh

benchmark-json: build
	./tests/benchmark/run.sh --json

lint:
	golangci-lint run

clean:
	rm -rf bin/
