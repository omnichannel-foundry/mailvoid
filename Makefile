SHELL := bash
.SHELLFLAGS := -eu -o pipefail -c
.ONESHELL:
.DELETE_ON_ERROR:
MAKEFLAGS += --warn-undefined-variables
MAKEFLAGS += --no-builtin-rules
MAKEFLAGS += --no-builtin-variables
.DEFAULT_GOAL := help

V ?= 0
ifeq ($(V),1)
  Q :=
else
  Q := @
endif

BINARY   := mailvoid
IT_BINARY := mailvoid-it
MODULE   := github.com/gnrfan/mailvoid
CMD      := ./cmd/$(BINARY)
IT_CMD   := ./cmd/$(IT_BINARY)
BUILDPKG := $(MODULE)/internal/build
OUTDIR   := bin
TARGET   := $(OUTDIR)/$(BINARY)
IT_TARGET := $(OUTDIR)/$(IT_BINARY)

PREFIX  ?= $(HOME)/.local
BINDIR  ?= $(PREFIX)/bin
DESTDIR ?=

GO      ?= go
GOFLAGS ?=
ARGS    ?=

ifeq ($(origin VERSION),undefined)
VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
endif
COMMIT := $(shell git rev-parse --short HEAD 2>/dev/null || echo none)
DATE   := $(shell date -u +%Y-%m-%dT%H:%M:%SZ)

LDFLAGS := -X $(BUILDPKG).Version=$(VERSION) \
           -X $(BUILDPKG).Commit=$(COMMIT) \
           -X $(BUILDPKG).Date=$(DATE)

SOURCES := $(shell find . -name '*.go' -not -path './vendor/*' 2>/dev/null)

##@ Build

.PHONY: build
build: $(TARGET) ## Build the binary into bin/

$(TARGET): $(SOURCES)
	$(Q)mkdir -p $(@D)
	$(Q)$(GO) build $(GOFLAGS) -trimpath -ldflags '$(LDFLAGS)' -o $@ $(CMD)
	$(Q)echo "built $@ ($(VERSION))"

.PHONY: release
release: ## Build a stripped static binary
	$(Q)mkdir -p $(OUTDIR)
	$(Q)CGO_ENABLED=0 $(GO) build $(GOFLAGS) -trimpath \
		-ldflags '-s -w $(LDFLAGS)' -o $(TARGET) $(CMD)
	$(Q)echo "built $(TARGET) ($(VERSION), stripped)"

.PHONY: run
run: $(TARGET) ## Build and run (make run ARGS='--http-addr :8025')
	$(Q)$(TARGET) $(ARGS)

.PHONY: it
it: $(IT_TARGET) ## Build the standalone integration runner
	$(Q)echo "built $(IT_TARGET)"

$(IT_TARGET): $(SOURCES)
	$(Q)mkdir -p $(@D)
	$(Q)$(GO) build $(GOFLAGS) -trimpath -o $@ $(IT_CMD)
	$(Q)echo "built $@"

.PHONY: integration
integration: $(TARGET) $(IT_TARGET) ## Run the JTD-driven integration suite in batch mode
	$(Q)$(IT_TARGET) -batch -server $(TARGET) -suite testdata/integration

.PHONY: integration-tui
integration-tui: $(TARGET) $(IT_TARGET) ## Run the integration suite in the interactive TUI
	$(Q)$(IT_TARGET) -server $(TARGET) -suite testdata/integration

##@ Quality

.PHONY: test
test: ## Run tests
	$(Q)$(GO) test $(ARGS) ./...

.PHONY: test-race
test-race: ## Run tests with the race detector, uncached
	$(Q)$(GO) test -race -count=1 ./...

.PHONY: cover
cover: ## Write and open an HTML coverage report
	$(Q)$(GO) test -coverprofile=coverage.out ./...
	$(Q)$(GO) tool cover -html=coverage.out

.PHONY: vet
vet: ## Run go vet
	$(Q)$(GO) vet ./...

.PHONY: fmt
fmt: ## Format code (gofumpt if available, else gofmt)
	$(Q)if command -v gofumpt >/dev/null 2>&1; then
		gofumpt -w .
	else
		gofmt -s -w .
	fi

.PHONY: fmt-check
fmt-check: ## Fail if any file is unformatted
	$(Q)out=$$(gofmt -l .)
	if [ -n "$$out" ]; then
		echo "unformatted files:" >&2
		echo "$$out" >&2
		exit 1
	fi

.PHONY: lint
lint: require-golangci-lint ## Run golangci-lint
	$(Q)golangci-lint run ./...

.PHONY: check
check: fmt-check vet test ## Run the checks CI runs

##@ Modules

.PHONY: tidy
tidy: ## Run go mod tidy
	$(Q)$(GO) mod tidy

.PHONY: tidy-check
tidy-check: ## Fail if go.mod/go.sum are not tidy
	$(Q)$(GO) mod tidy -diff

##@ Docs

DOCS_DIR    := docs
DOCS_PUBLIC := $(DOCS_DIR)/public

.PHONY: docs
docs: require-hugo ## Build the Hugo Book documentation site
	$(Q)hugo --source $(DOCS_DIR) --destination public --minify
	$(Q)echo "built $(DOCS_PUBLIC)"

.PHONY: docs-serve
docs-serve: require-hugo ## Serve docs at http://localhost:1313
	$(Q)hugo server --source $(DOCS_DIR) --minify

.PHONY: docs-clean
docs-clean: ## Remove generated documentation
	$(Q)rm -rf $(DOCS_PUBLIC) $(DOCS_DIR)/resources $(DOCS_DIR)/.hugo_build.lock

##@ Install

.PHONY: install
install: $(TARGET) ## Install the binary into PREFIX/bin
	$(Q)install -D -m 0755 $(TARGET) $(DESTDIR)$(BINDIR)/$(BINARY)
	$(Q)echo "installed $(DESTDIR)$(BINDIR)/$(BINARY)"

.PHONY: uninstall
uninstall: ## Remove the installed binary
	$(Q)rm -f $(DESTDIR)$(BINDIR)/$(BINARY)

.PHONY: clean
clean: ## Remove build artifacts
	$(Q)rm -rf $(OUTDIR) coverage.out
	$(Q)rm -f $(BINARY)

##@ Help

.PHONY: help
help: ## Show this help
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m<target>\033[0m\n"} \
		/^[a-zA-Z0-9_%\/-]+:.*?##/ { printf "  \033[36m%-18s\033[0m %s\n", $$1, $$2 } \
		/^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) }' $(MAKEFILE_LIST)
	@echo ""

.PHONY: require-%
require-%:
	@command -v $* >/dev/null 2>&1 || { \
		echo "error: '$*' is required but not installed" >&2; exit 1; }
