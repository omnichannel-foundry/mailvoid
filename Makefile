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
LOREM_BINARY := lorem-mail-generator
MODULE   := github.com/gnrfan/mailvoid
CMD      := ./cmd/$(BINARY)
IT_CMD   := ./cmd/$(IT_BINARY)
LOREM_CMD := ./cmd/$(LOREM_BINARY)
BUILDPKG := $(MODULE)/internal/build
OUTDIR   := bin
TARGET   := $(OUTDIR)/$(BINARY)
IT_TARGET := $(OUTDIR)/$(IT_BINARY)
LOREM_TARGET := $(OUTDIR)/$(LOREM_BINARY)

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
WEB_ASSETS := $(shell find internal/web/static internal/web/templates -type f 2>/dev/null)

##@ Build

.PHONY: build
build: $(TARGET) ## Build the binary into bin/

$(TARGET): $(SOURCES) $(WEB_ASSETS)
	$(Q)mkdir -p $(@D)
	$(Q)$(GO) build $(GOFLAGS) -trimpath -ldflags '$(LDFLAGS)' -o $@ $(CMD)
	$(Q)echo "built $@ ($(VERSION))"

.PHONY: assets
assets: ## Rebuild the Tailwind + DaisyUI stylesheet
	$(Q)npm ci
	$(Q)npm run build:css
	$(Q)echo "built internal/web/static/app.css"

.PHONY: assets-watch
assets-watch: ## Watch templates and rebuild the stylesheet
	$(Q)npm run watch:css

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

.PHONY: lorem
lorem: $(LOREM_TARGET) ## Build the random lorem mail generator

$(LOREM_TARGET): $(SOURCES)
	$(Q)mkdir -p $(@D)
	$(Q)$(GO) build $(GOFLAGS) -trimpath -o $@ $(LOREM_CMD)
	$(Q)echo "built $@"

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
DOCS_EN_DIR := $(DOCS_DIR)/en
DOCS_ES_DIR := $(DOCS_DIR)/es

.PHONY: docs
docs: require-hugo ## Build both Hugo Book documentation sites
	$(Q)$(MAKE) -C $(DOCS_DIR) all
	$(Q)echo "built $(DOCS_EN_DIR)/public and $(DOCS_ES_DIR)/public"

.PHONY: docs-en
docs-en: require-hugo ## Build the English documentation site
	$(Q)hugo --source $(DOCS_EN_DIR) --minify
	$(Q)echo "built $(DOCS_EN_DIR)/public"

.PHONY: docs-es
docs-es: require-hugo ## Build the Spanish documentation site
	$(Q)hugo --source $(DOCS_ES_DIR) --minify
	$(Q)echo "built $(DOCS_ES_DIR)/public"

.PHONY: docs-serve-en
docs-serve-en: require-hugo ## Serve English docs at http://localhost:1313/en/
	$(Q)hugo server --source $(DOCS_EN_DIR) --baseURL http://localhost:1313/en/ --minify

.PHONY: docs-serve-es
docs-serve-es: require-hugo ## Serve Spanish docs at http://localhost:1313/es/
	$(Q)hugo server --source $(DOCS_ES_DIR) --baseURL http://localhost:1313/es/ --minify

.PHONY: docs-clean
docs-clean: ## Remove generated documentation
	$(Q)rm -rf $(DOCS_EN_DIR)/public $(DOCS_ES_DIR)/public
	$(Q)rm -rf $(DOCS_EN_DIR)/resources $(DOCS_ES_DIR)/resources
	$(Q)rm -f $(DOCS_EN_DIR)/.hugo_build.lock $(DOCS_ES_DIR)/.hugo_build.lock

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
