# partner-api Makefile (W0 scaffold).
SHELL := /bin/bash
.SHELLFLAGS := -eu -o pipefail -c
.DEFAULT_GOAL := help

BIN_DIR := bin
SERVER_BIN := $(BIN_DIR)/partner-api

# 默认本地 docker-compose DSN（与 docker-compose.dev.yml 一致）。
LOCAL_BIZ_DSN ?= mysql://tnbiz_app:tnbiz_app_pw@tcp(127.0.0.1:3306)/partner_db?multiStatements=true&parseTime=true&charset=utf8mb4&collation=utf8mb4_0900_ai_ci
LOCAL_LOG_DSN ?= mysql://tnbiz_app:tnbiz_app_pw@tcp(127.0.0.1:3306)/log_db?multiStatements=true&parseTime=true

.PHONY: help
help:
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | awk 'BEGIN{FS=":.*?## "};{printf "  \033[36m%-20s\033[0m %s\n",$$1,$$2}'

.PHONY: tidy
tidy: ## go mod tidy
	go mod tidy

.PHONY: build
build: ## 编译 partner-api 二进制
	@mkdir -p $(BIN_DIR)
	CGO_ENABLED=0 go build -trimpath -o $(SERVER_BIN) ./cmd/server

.PHONY: dev
dev: ## 本地起 partner-api（默认监听 8080）
	go run ./cmd/server

.PHONY: test
test: ## go test（骨架单测）
	go test ./... -count=1

.PHONY: lint
lint: ## golangci-lint（含 bola-scope-required 占位）
	@which golangci-lint > /dev/null || (echo "install: https://golangci-lint.run" && exit 1)
	golangci-lint run ./...

.PHONY: vet
vet:
	go vet ./...

.PHONY: migrate-up
migrate-up: ## 跑 migrations（需要 golang-migrate）
	@which migrate > /dev/null || (echo "install: https://github.com/golang-migrate/migrate" && exit 1)
	migrate -path migrations -database "$(LOCAL_BIZ_DSN)" up

.PHONY: migrate-down
migrate-down:
	migrate -path migrations -database "$(LOCAL_BIZ_DSN)" down 1

.PHONY: migrate-create
migrate-create: ## 用法: make migrate-create name=add_xxx
	migrate create -ext sql -dir migrations -seq $(name)

.PHONY: sbom
sbom: ## syft 生成 SBOM
	@which syft > /dev/null || (echo "install: https://github.com/anchore/syft" && exit 1)
	syft . -o cyclonedx-json=sbom-api.json

.PHONY: sign
sign: ## cosign 签名（需 COSIGN_KEY）
	@which cosign > /dev/null || (echo "install: https://github.com/sigstore/cosign" && exit 1)
	@echo "TODO(ops): cosign sign --key $$COSIGN_KEY <image>"

.PHONY: clean
clean:
	rm -rf $(BIN_DIR) sbom-api.json coverage.txt
