# TraceNex Partner monorepo Makefile (W0 scaffold)
#
# Targets:
#   make help          show this help
#   make dev           start docker-compose.dev + partner-api hot reload + 4 web dev servers
#   make build         build partner-api binary + 4 web dist
#   make test          run go test ./... + pnpm test
#   make lint          go vet + golangci-lint + pnpm lint
#   make migrate-up    apply DB migrations to local MySQL
#   make migrate-down  roll back last migration
#   make sbom          generate SBOM via syft
#   make sign          cosign sign images (CI helper)
#   make clean         remove build artifacts

API_DIR := apps/partner-api
WEB_APPS := partner-web-storefront partner-web-customer partner-web-partner partner-web-admin

.PHONY: help
help:
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-20s\033[0m %s\n", $$1, $$2}'

# ---------------------------------------------------------------------------
# Development
# ---------------------------------------------------------------------------

.PHONY: dev
dev: ## Start full local stack (docker-compose + api + web)
	docker compose -f docker-compose.dev.yml up -d
	@echo "Local stack up. Run partner-api: make api-dev. Web: pnpm -r dev"

.PHONY: api-dev
api-dev: ## Run partner-api against local stack
	cd $(API_DIR) && go run ./cmd/server

.PHONY: web-dev
web-dev: ## Run all 4 web dev servers in parallel
	pnpm -r --parallel --filter './apps/partner-web-*' dev

# ---------------------------------------------------------------------------
# Build
# ---------------------------------------------------------------------------

.PHONY: build
build: build-api build-web ## Build partner-api binary + 4 web dist

.PHONY: build-api
build-api: ## Build partner-api Go binary into apps/partner-api/bin/
	cd $(API_DIR) && CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o bin/partner-api ./cmd/server

.PHONY: build-web
build-web: ## Build all 4 React apps
	pnpm install --frozen-lockfile=false
	pnpm -r --filter './apps/partner-web-*' build

# ---------------------------------------------------------------------------
# Test
# ---------------------------------------------------------------------------

.PHONY: test
test: test-api test-web ## Run all tests

.PHONY: test-api
test-api: ## Go unit + repository tests
	cd $(API_DIR) && go test ./... -race -coverprofile=coverage.out

.PHONY: test-web
test-web: ## Vitest unit tests for all web apps
	pnpm -r --filter './apps/partner-web-*' test

# ---------------------------------------------------------------------------
# Lint
# ---------------------------------------------------------------------------

.PHONY: lint
lint: lint-api lint-web ## Run all linters

.PHONY: lint-api
lint-api: ## go vet + golangci-lint
	cd $(API_DIR) && go vet ./...
	cd $(API_DIR) && (command -v golangci-lint >/dev/null 2>&1 && golangci-lint run ./... || echo "golangci-lint not installed; skipping")

.PHONY: lint-web
lint-web: ## eslint + tsc --noEmit
	pnpm -r --filter './apps/partner-web-*' lint
	pnpm -r --filter './apps/partner-web-*' typecheck

# ---------------------------------------------------------------------------
# Migrations
# ---------------------------------------------------------------------------

DB_DSN ?= mysql://tnbiz_app:tnbiz_app@tcp(127.0.0.1:3306)/partner_db?parseTime=true&multiStatements=true

.PHONY: migrate-up
migrate-up: ## Apply DB migrations
	cd $(API_DIR) && (command -v migrate >/dev/null 2>&1 && \
		migrate -path migrations -database "$(DB_DSN)" up || \
		echo "golang-migrate not installed; install via: go install -tags 'mysql' github.com/golang-migrate/migrate/v4/cmd/migrate@latest")

.PHONY: migrate-down
migrate-down: ## Roll back the last migration
	cd $(API_DIR) && migrate -path migrations -database "$(DB_DSN)" down 1

.PHONY: migrate-status
migrate-status:
	cd $(API_DIR) && migrate -path migrations -database "$(DB_DSN)" version

# ---------------------------------------------------------------------------
# Supply chain
# ---------------------------------------------------------------------------

.PHONY: sbom
sbom: ## Generate CycloneDX SBOM via syft
	@command -v syft >/dev/null 2>&1 || { echo "install syft first"; exit 1; }
	syft packages dir:$(API_DIR) -o cyclonedx-json > sbom-partner-api.json
	syft packages dir:. -o cyclonedx-json > sbom-monorepo.json

.PHONY: sign
sign: ## cosign sign last-built image (requires COSIGN_KEY)
	@command -v cosign >/dev/null 2>&1 || { echo "install cosign first"; exit 1; }
	cosign sign --yes $(IMAGE_REF)

# ---------------------------------------------------------------------------
# Clean
# ---------------------------------------------------------------------------

.PHONY: clean
clean:
	rm -rf $(API_DIR)/bin $(API_DIR)/coverage.out
	pnpm -r --filter './apps/partner-web-*' exec rm -rf dist || true

.DEFAULT_GOAL := help
