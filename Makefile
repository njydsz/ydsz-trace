# Ydsz Trace - Unified Build & Dev Commands
#
# Usage:
#   make all          — Build all modules (pkg, logs, logc)
#   make logs         — Build logs server
#   make logc         — Build logc agent
#   make test         — Run Go tests across all modules
#   make lint         — Run go vet across all modules (golangci-lint optional)
#   make clean        — Clean build artifacts
#   make run-logs     — Run logs server (default port from config)
#   make run-logc     — Run logc agent (default port from config)
#   make run-logc-docker — Run logc in docker mode (auto-discovers containers)
#   make run-logc-k8s    — Run logc in k8s mode (auto-discovers pods)
#   make docker-all   — Build Docker images for logs + logc
#   make deploy-docker-file   — Deploy with docker compose in file mode
#   make deploy-docker-docker — Deploy with docker compose in docker mode
#
# Environment variables for modes:
#   YDSZ_LOG_SOURCE=file|docker|k8s  (default: file)

SHELL := /bin/sh
GO    ?= go
PKG   ?= pkg logs logc

.PHONY: all build test lint clean fmt release

all: deps build

# ==================== Dependencies ====================
deps:
	$(GO) mod download
	@cd pkg && $(GO) mod download 2>/dev/null || true
	@cd logs && $(GO) mod download 2>/dev/null || true
	@cd logc && $(GO) mod download 2>/dev/null || true

# ==================== Build ====================
build: build-pkg build-logs build-logc

build-pkg:
	cd pkg && $(GO) build ./...

build-logs:
	cd logs && $(GO) build -o ../bin/logs ./...

build-logc:
	cd logc && $(GO) build -o ../bin/logc ./...

# ==================== Test ====================
test: test-pkg test-logs test-logc

test-pkg:
	cd pkg && $(GO) test -race -cover ./...

test-logs:
	cd logs && $(GO) test -race ./...

test-logc:
	cd logc && $(GO) test -race ./...

test-verbose:
	@for m in $(PKG); do \
		echo "=== Testing $$m ==="; \
		cd $$m && $(GO) test -v -race ./... 2>/dev/null; cd ..; \
	done

# ==================== Lint / Format ====================
lint:
	@for m in $(PKG); do \
		echo "=== Vetting $$m ==="; \
		cd $$m && $(GO) vet ./... 2>&1 || true; cd ..; \
	done

lint-install:
	@command -v golangci-lint >/dev/null 2>&1 || \
		(echo "Installing golangci-lint..." && \
			go install github.com/golangci/golangci-lint/v6/golangci-lint@v1.62.0 2>/dev/null || true)

fmt:
	@for m in $(PKG); do \
		echo "=== Formatting $$m ==="; \
		cd $$m && $(GO) fmt ./...; cd ..; \
	done

# ==================== Dev Run ====================
run-logs:
	cd logs && $(GO) run .

run-logc:
	cd logc && $(GO) run .

# Docker mode：自动发现容器
run-logc-docker:
	cd logc && YDSZ_LOG_SOURCE=docker YDSZ_LOG_SERVER=127.0.0.1:2021 $(GO) run .

# K8s 模式：自动发现 Pod
run-logc-k8s:
	cd logc && YDSZ_LOG_SOURCE=k8s YDSZ_LOG_SERVER=127.0.0.1:2021 $(GO) run .

# ==================== Frontend ====================
frontend:
	cd web && npm install && npm run build

frontend-dev:
	cd web && npm install && npm run dev

# ==================== Docker Build ====================
docker-logs:
	cd logs && docker build -t ydsz-trace/logs:latest .

docker-logc:
	cd logc && docker build -t ydsz-trace/logc:latest .

docker-all: docker-logs docker-logc

# ==================== Docker Compose Deploy ====================
deploy-docker-file:
	docker compose -f deploy/docker/docker-compose.multi-mode.yml --profile file up -d

deploy-docker-docker:
	docker compose -f deploy/docker/docker-compose.multi-mode.yml --profile docker up -d

deploy-docker-down:
	docker compose -f deploy/docker/docker-compose.multi-mode.yml --profile file --profile docker down

# ==================== K8s Deploy ====================
deploy-k8s:
	kubectl apply -f deploy/k8s/rbac.yaml
	kubectl apply -f deploy/k8s/logs-deployment.yaml
	kubectl apply -f deploy/k8s/logc-daemonset.yaml

remove-k8s:
	kubectl delete -f deploy/k8s/logc-daemonset.yaml
	kubectl delete -f deploy/k8s/logs-deployment.yaml
	kubectl delete -f deploy/k8s/rbac.yaml

# ==================== Release ====================
release: build
	mkdir -p dist
	cp bin/logs dist/
	cp bin/logc dist/
	@echo "Release artifacts in dist/"

# ==================== Clean ====================
clean:
	rm -rf bin/ dist/
	find . -name '*.test' -delete
	find . -name 'coverage.out' -delete
	@for m in $(PKG); do \
		cd $$m && $(GO) clean 2>/dev/null; cd ..; \
	done

# ==================== CI ====================
ci: lint test build
	@echo "CI pipeline complete"
