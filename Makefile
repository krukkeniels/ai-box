# Root Makefile for AI-Box — coordinates all 4 Go binaries.

VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
COMMIT  ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo "none")
DATE    ?= $(shell date -u +"%Y-%m-%dT%H:%M:%SZ")

LDFLAGS := -X main.version=$(VERSION) -X main.commit=$(COMMIT) -X main.date=$(DATE)
export CGO_ENABLED := 0

BINDIR := bin

# Module directories (each has its own go.mod).
MODULES := cmd/aibox cmd/aibox-credential-helper cmd/aibox-llm-proxy cmd/aibox-git-remote-helper

# Binary names derived from module dirs.
BINARIES := aibox aibox-credential-helper aibox-llm-proxy aibox-git-remote-helper

# Cross-compile targets for release-local.
PLATFORMS := linux/amd64 linux/arm64 darwin/amd64 darwin/arm64 windows/amd64

.PHONY: build $(addprefix build-,$(BINARIES)) test lint fmt release-local clean install

# --- Build all binaries ---

build: $(addprefix build-,$(BINARIES))

build-aibox:
	cd cmd/aibox && go build -ldflags '$(LDFLAGS)' -o ../../$(BINDIR)/aibox ./

build-credential-helper:
	cd cmd/aibox-credential-helper && go build -ldflags '$(LDFLAGS)' -o ../../$(BINDIR)/aibox-credential-helper ./

build-llm-proxy:
	cd cmd/aibox-llm-proxy && go build -ldflags '$(LDFLAGS)' -o ../../$(BINDIR)/aibox-llm-proxy ./

build-git-remote-helper:
	cd cmd/aibox-git-remote-helper && go build -ldflags '$(LDFLAGS)' -o ../../$(BINDIR)/aibox-git-remote-helper ./

# --- Test all modules ---

test:
	@for mod in $(MODULES); do \
		echo "==> Testing $$mod"; \
		(cd $$mod && go test ./...) || exit 1; \
	done

# --- Lint all modules ---

lint:
	@for mod in $(MODULES); do \
		echo "==> Linting $$mod"; \
		(cd $$mod && golangci-lint run ./...) || exit 1; \
	done

# --- Format all modules ---

fmt:
	@for mod in $(MODULES); do \
		echo "==> Formatting $$mod"; \
		gofmt -w $$mod; \
		goimports -w $$mod; \
	done

# --- Cross-compile for all platforms ---

release-local:
	@for platform in $(PLATFORMS); do \
		GOOS=$${platform%/*}; \
		GOARCH=$${platform#*/}; \
		suffix=""; \
		if [ "$$GOOS" = "windows" ]; then suffix=".exe"; fi; \
		for mod in $(MODULES); do \
			bin=$${mod##*/}; \
			echo "==> $$GOOS/$$GOARCH $$bin"; \
			(cd $$mod && GOOS=$$GOOS GOARCH=$$GOARCH go build \
				-ldflags '$(LDFLAGS)' \
				-o ../../$(BINDIR)/$$GOOS-$$GOARCH/$$bin$$suffix ./) || exit 1; \
		done; \
	done

# --- Install to ~/.local/bin ---

install: build
	@mkdir -p $(HOME)/.local/bin
	@for bin in $(BINARIES); do \
		cp $(BINDIR)/$$bin $(HOME)/.local/bin/$$bin; \
		echo "Installed $$bin -> $(HOME)/.local/bin/$$bin"; \
	done

# --- Clean ---

clean:
	rm -rf $(BINDIR)/
