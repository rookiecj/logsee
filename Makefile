# logsee — stdio log TUI (see docs/plans/stdio-log-viewer-prd.md)
BINARY ?= logsee
OUTDIR ?= bin
OUT    := $(OUTDIR)/$(BINARY)

VERSION_FILE ?= VERSION
VERSION := $(strip $(shell test -f $(VERSION_FILE) && tr -d '\n\r' < $(VERSION_FILE) || echo ""))
LDFLAGS = $(if $(VERSION),-X 'git.inpt.fr/42dottools/log/internal/version.AppVersion=$(VERSION)',)

.PHONY: all help build test run fmt fmt-check vet lint publish publish-verify clean tidy dep

all: build test

help:
	@echo "Targets:"
	@echo "  make build   - go build (VERSION from $(VERSION_FILE) via -ldflags) -> $(OUT)"
	@echo "  make test    - go test ./..."
	@echo "  make run     - build then run $(OUT) (ARGS='file.log' or pipe stdin)"
	@echo "  make fmt     - go fmt ./..."
	@echo "  make fmt-check - fail if gofmt would change any .go file"
	@echo "  make vet     - go vet ./..."
	@echo "  make lint    - same as vet (release gate)"
	@echo "  make publish-verify - fmt-check, lint, test, build (must pass before publish)"
	@echo "  make publish [BUMP=minor|patch|major] - bump VERSION (default: minor), commit, tag, push"
	@echo "  make tidy    - go mod tidy"
	@echo "  make dep     - go mod download && go mod verify"
	@echo "  make clean   - remove $(OUTDIR)/"

build:
	@mkdir -p $(OUTDIR)
	go build -ldflags "$(LDFLAGS)" -o $(OUT) ./cmd/$(BINARY)

test:
	go test ./...

dev:
	go run -ldflags "$(LDFLAGS)" ./cmd/$(BINARY) $(ARGS)

run: build
	$(OUT) $(ARGS)

fmt:
	go fmt ./...

# Fail if any .go file is not gofmt-formatted (run: make fmt).
fmt-check:
	@test -z "$$(gofmt -l $$(find . -name '*.go' ! -path './.git/*'))" || (echo "gofmt needed: run make fmt"; exit 1)

vet:
	go vet ./...

lint: vet

publish-verify: fmt-check lint test build

# BUMP: minor (default, x.y+1.0), patch (x.y.z+1), major (x+1.0.0)
publish:
	@bash "$(CURDIR)/scripts/publish.sh" "$(BUMP)"

tidy:
	go mod tidy

dep:
	go mod download
	go mod verify

clean:
	rm -rf $(OUTDIR)
