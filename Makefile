# Ydsz Trace - Unified Build & Dev Commands
#
# Usage:
#   make all          — Build all modules (pkg, logs, logc)
#   make logs         — Build logs server
#   make logc         — Build logc agent
#   make test         — Run Go tests across all modules
#   make test-pkg     — Run tests for pkg modules
#   make test-logs    — Run tests for logs module
#   make test-logc    — Run tests for logc module
#   make lint         — Run go vet across all modules
#   make clean        — Clean build artifacts
#   make run-logs     — Run logs server (default port from config)
#   make run-logc     — Run logc agent (default port from config)
#   make deps         — Download Go dependencies
#   make frontend     — Build frontend (requires node/npm)
#
# Note: golang.org/x/exp dependency may need network access for first build.

SHELL := /bin/sh
GO    ?= go
PKG   ?= pkg logs logc

.PHONY: all test lint clean

all: deps build

# Dependencies
deps:
	$(GO) mod download
	@cd pkg && $(GO) mod download 2>/dev/null || true
	@cd logs && $(GO) mod download 2>/dev/null || true
	@cd logc && $(GO) mod download 2>/dev/null || true

# Build
build: build-pkg build-logs build-logc

build-pkg:
	cd pkg && $(GO) build ./...

build-logs:
	cd logs && $(GO) build -o ../bin/logs ./...

build-logc:
	cd logc && $(GO) build -o ../bin/logc ./...

# Test
test: test-pkg test-logs test-logc

test-pkg:
	cd pkg && $(GO) test -race -cover ./...

test-logs:
	cd logs && $(GO) test -race -cover ./... 2>/dev/null || cd logs && $(GO) test -race ./...

test-logc:
	cd logc && $(GO) test -race -cover ./... 2>/dev/null || cd logc && $(GO) test -race ./...

test-verbose:
	@for m in $(PKG); do \
		echo "=== Testing $$m ==="; \
		cd $$m && $(GO) test -v -race ./... 2>/dev/null; cd ..; \
	done

# Lint
lint:
	@for m in $(PKG); do \
		echo "=== Vetting $$m ==="; \
		cd $$m && $(GO) vet ./... 2>&1 || true; cd ..; \
	done

fmt:
	@for m in $(PKG); do \
		echo "=== Formatting $$m ==="; \
		cd $$m && $(GO) fmt ./...; cd ..; \
	done

# Run
run-logs:
	cd logs && $(GO) run .

run-logc:
	cd logc && $(GO) run .

# Frontend
frontend:
	cd web && npm install && npm run build

frontend-dev:
	cd web && npm install && npm run dev

# Docker
docker-logs:
	cd logs && docker build -t ydsz-trace/logs:latest .

docker-logc:
	cd logc && docker build -t ydsz-trace/logc:latest .

# Release artifacts
release: build
	mkdir -p dist
	cp bin/logs dist/
	cp bin/logc dist/
	@echo "Release artifacts in dist/"

# Clean
clean:
	rm -rf bin/ dist/
	find . -name '*.test' -delete
	find . -name 'coverage.out' -delete
	@for m in $(PKG); do \
		cd $$m && $(GO) clean 2>/dev/null; cd ..; \
	done

# CI target (lint + test + build)
ci: lint test build
	@echo "CI pipeline complete"
