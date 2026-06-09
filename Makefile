.DEFAULT_GOAL := help
SHELL := /bin/bash

FRONTEND_DIR := frontend
GO_FILES := $(shell find . -name '*.go' -not -path './frontend/*' -not -path './build/*' -not -path './bindings/*' 2>/dev/null)

.PHONY: help
help: ## Show this help
	@awk 'BEGIN {FS = ":.*##"; printf "Usage: make \033[36m<target>\033[0m\n\nTargets:\n"} \
	  /^[a-zA-Z_.-]+:.*?##/ { printf "  \033[36m%-20s\033[0m %s\n", $$1, $$2 }' $(MAKEFILE_LIST)

## --- Dev ---

.PHONY: dev
dev: ## Run Wails v3 dev server (hot-reload)
	wails3 dev -config ./build/config.yml

.PHONY: build
build: ## Build the production app
	wails3 task build

.PHONY: bindings
bindings: ## Generate Go <-> JS bindings
	wails3 generate bindings

.PHONY: sqlc
sqlc: ## Regenerate sqlc Go from internal/db/queries
	sqlc generate

## --- Deps ---

.PHONY: deps
deps: deps.go deps.frontend ## Install all dependencies

.PHONY: deps.go
deps.go: ## Tidy Go modules
	go mod tidy

.PHONY: deps.frontend
deps.frontend: ## Install npm deps (root + frontend)
	npm install
	cd $(FRONTEND_DIR) && npm install

## --- Format / Lint ---

.PHONY: fmt
fmt: fmt.go fmt.frontend ## Format everything

.PHONY: fmt.go
fmt.go: ## Format Go (gofmt + goimports)
	gofmt -w $(GO_FILES)
	goimports -w -local github.com/mandloideep/miniclaw $(GO_FILES)

.PHONY: fmt.frontend
fmt.frontend: ## Format frontend with Biome
	cd $(FRONTEND_DIR) && npm run check

.PHONY: lint
lint: lint.go lint.frontend ## Lint everything

.PHONY: lint.go
lint.go: ## Run golangci-lint
	golangci-lint run ./...

.PHONY: lint.frontend
lint.frontend: ## Run Biome lint
	cd $(FRONTEND_DIR) && npm run lint

## --- Test ---

.PHONY: test
test: ## Run Go tests with race + coverage
	go test -race -coverprofile=coverage.out ./...

.PHONY: test.cover
test.cover: test ## Open coverage report in browser
	go tool cover -html=coverage.out

## --- Ollama ---

.PHONY: ollama.up
ollama.up: ## Start local Ollama (docker)
	docker compose up -d ollama

.PHONY: ollama.down
ollama.down: ## Stop local Ollama
	docker compose down

.PHONY: ollama.logs
ollama.logs: ## Tail Ollama logs
	docker compose logs -f ollama

## --- Clean ---

.PHONY: clean
clean: ## Remove build artifacts
	rm -rf bin dist coverage.out coverage.html
	rm -rf $(FRONTEND_DIR)/dist $(FRONTEND_DIR)/bindings
