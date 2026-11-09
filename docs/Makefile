SHELL := bash
.SHELLFLAGS := -eu -o pipefail -c
.ONESHELL:
.DELETE_ON_ERROR:
.DEFAULT_GOAL := help

HUGO ?= hugo

.PHONY: help
help: ## Show this help
	@awk 'BEGIN {FS = ":.*## "} /^[a-zA-Z_-]+:.*## / {printf "%-14s %s\n", $$1, $$2}' $(MAKEFILE_LIST)

.PHONY: en
en: ## Build the English docs into docs/en/public
	$(HUGO) --source en --minify

.PHONY: es
es: ## Build the Spanish docs into docs/es/public
	$(HUGO) --source es --minify

.PHONY: all
all: en es ## Build both language sites

.PHONY: serve-en
serve-en: ## Serve the English docs at http://localhost:1313/en/
	$(HUGO) --source en server --baseURL http://localhost:1313/en/

.PHONY: serve-es
serve-es: ## Serve the Spanish docs at http://localhost:1313/es/
	$(HUGO) --source es server --baseURL http://localhost:1313/es/

.PHONY: clean
clean: ## Remove generated output
	rm -rf en/public es/public en/resources es/resources
