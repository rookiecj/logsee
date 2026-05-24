.DEFAULT_GOAL := help

GO ?= go
GOFMT ?= gofmt

BIN_DIR := bin
BINARY := logsee
LOCAL_BIN ?= $(HOME)/.local/bin
MAIN_PKG := ./cmd/logsee
GO_PACKAGES := ./...
GO_SOURCE_DIRS := cmd internal
VERSION_FILE := VERSION
VERSION := $(shell tr -d '[:space:]' < $(VERSION_FILE))
VERSION_PART ?= minor
GO_LDFLAGS ?=
BUILD_LDFLAGS := $(strip $(GO_LDFLAGS) -X main.version=$(VERSION))

.PHONY: help build publish-local run test test-once fmt fmt-check lint tidy clean version version-up

help: ## Show available targets.
	@awk 'BEGIN {FS = ":.*## "; printf "Usage: make <target>\\n\\nTargets:\\n"} /^[a-zA-Z0-9_-]+:.*## / {printf "  %-12s %s\\n", $$1, $$2}' $(MAKEFILE_LIST)

build: $(VERSION_FILE) ## Build the logsee binary into bin/.
	@mkdir -p $(BIN_DIR)
	$(GO) build -ldflags "$(BUILD_LDFLAGS)" -o $(BIN_DIR)/$(BINARY) $(MAIN_PKG)

publish-local: build ## Install the binary to ~/.local/bin.
	@mkdir -p $(LOCAL_BIN)
	install -m 0755 $(BIN_DIR)/$(BINARY) $(LOCAL_BIN)/$(BINARY)

run: ## Run logsee with optional ARGS="..." arguments.
	$(GO) run $(MAIN_PKG) $(ARGS)

test: ## Run all tests.
	$(GO) test $(GO_PACKAGES)

test-once: ## Run all tests without cache.
	$(GO) test $(GO_PACKAGES) -count=1

fmt: ## Format Go source files.
	$(GOFMT) -w $(GO_SOURCE_DIRS)

fmt-check: ## Check Go formatting without modifying files.
	@test -z "$$($(GOFMT) -l $(GO_SOURCE_DIRS))"

lint: fmt-check ## Run lightweight lint checks.
	$(GO) vet $(GO_PACKAGES)

tidy: ## Tidy Go module dependencies.
	$(GO) mod tidy

version: ## Show the version used for binary builds.
	@printf '%s\n' '$(VERSION)'

version-up: $(VERSION_FILE) ## Bump VERSION. Defaults to minor; set VERSION_PART=major|minor|patch.
	@version="$$(tr -d '[:space:]' < $(VERSION_FILE))"; \
	IFS=.; set -- $$version; IFS=' '; \
	if [ "$$#" -ne 3 ]; then echo "invalid VERSION: $$version" >&2; exit 2; fi; \
	major="$$1"; minor="$$2"; patch="$$3"; \
	case "$$major.$$minor.$$patch" in \
		*[!0-9.]*|.*|*..*|*.) echo "invalid VERSION: $$version" >&2; exit 2 ;; \
	esac; \
	case "$(VERSION_PART)" in \
		major) major=$$((major + 1)); minor=0; patch=0 ;; \
		minor) minor=$$((minor + 1)); patch=0 ;; \
		patch) patch=$$((patch + 1)) ;; \
		*) echo "unsupported VERSION_PART: $(VERSION_PART)" >&2; exit 2 ;; \
	esac; \
	next="$$major.$$minor.$$patch"; \
	printf '%s\n' "$$next" > $(VERSION_FILE); \
	printf '%s -> %s\n' "$$version" "$$next"

clean: ## Remove generated build artifacts.
	rm -rf $(BIN_DIR)
