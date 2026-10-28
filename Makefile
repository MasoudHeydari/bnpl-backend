# Auto-load .env so every Make target sees the variables.
-include .env
export

.DEFAULT_GOAL := build-development

# -----------------------------------------------------------------------------
# Variables
# -----------------------------------------------------------------------------

# Application / image
APP_NAME := bnpl
IMAGE_NAME ?= bnpl-backend
IMAGE_VERSION ?= v0.1.0
ENTRYPOINT ?= scripts/entrypoint.sh

# Hamravesh container registry publish
HAMRAVESH_CONTAINER_REGISTRY ?= registry.hamdocker.ir
HAMRAVESH_REGISTRY_NAMESPACE ?= parstis-technology
HAMRAVESH_IMAGE_LOCAL_TAG := $(IMAGE_NAME):$(IMAGE_VERSION)
HAMRAVESH_IMAGE_REMOTE := $(HAMRAVESH_CONTAINER_REGISTRY)/$(HAMRAVESH_REGISTRY_NAMESPACE)/$(IMAGE_NAME):$(IMAGE_VERSION)

# Local Postgres container / volume names (clean-db)
DB_CONTAINER_NAME ?= bnpl-db
DB_DOCKER_VOLUME_NAME ?= bnpl-backend_postgres_data_bnpl

# Protobuf layout
PROTO_DIR := proto
PROTO_GEN_DIR := gen/proto
PROTO_FILES := $(shell find $(PROTO_DIR) -name '*.proto')

# Tool versions / runners
GOLANGCI_LINT_VERSION ?= v2.12.2
MOCKERY_VERSION ?= v3.7.1
STATICCHECK_VERSION ?= 2025.1.1
SWAG_VERSION ?= v1.16.6
GOTESTSUM_VERSION ?= v1.13.0
GOTESTSUM ?= gotestsum
GOTESTSUM_FORMAT ?= pkgname

# Swag OpenAPI generation
SWAG_ENTRYPOINT ?= main.go
SWAG_OUTPUT_DIR ?= gen/swagger
# Search the entrypoint tree plus all of ./pkg so REST DTOs and nested model/
# usecase schema types resolve without enumerating individual packages.
# Generated output under gen/ is outside this scope and is not scanned.
SWAG_SEARCH_DIRS ?= ./cmd/bnpl,./pkg

# Docker Compose / Podman for local infra and integration tests
COMPOSE_FILE ?= docker-compose.local.yml
DOCKER_HOST_PODMAN ?= unix://$(XDG_RUNTIME_DIR)/podman/podman.sock

# Trusted customer policy-assignment import defaults
POLICY_ASSIGNMENT_FILE ?= assets/csv/trusted-subjects/trusted-subjects.csv
POLICY_ASSIGNMENT_ACTOR ?= dev.masoud1997@gmail.com

.PHONY: all
all: build

# -----------------------------------------------------------------------------
# Build / Run
# -----------------------------------------------------------------------------

GO_BUILD_TAGS := go_json nomsgpack
DEVELOPMENT_BUILD_TAGS := $(GO_BUILD_TAGS) development
PRODUCTION_BUILD_TAGS := $(GO_BUILD_TAGS) production

# Explicit package roots avoid accidental unreadable top-level directories
# (for example a leftover ./c) breaking `go test ./...` discovery.
GO_TEST_PACKAGES := ./cmd/... ./pkg/... ./internal/... ./gen/...

.PHONY: build
# Backwards-compatible local build. Production builds must always use build-production.
build: build-development

.PHONY: build-development
build-development: mkdir-bin
	CGO_ENABLED=0 GOPROXY=https://goproxy.io,direct go build \
		-tags="$(DEVELOPMENT_BUILD_TAGS)" \
		-trimpath \
		-ldflags='-s -w -extldflags "-static"' \
		-o build/bin/$(APP_NAME) ./cmd/bnpl

.PHONY: build-production
build-production: mkdir-bin
	CGO_ENABLED=0 GOPROXY=https://goproxy.io,direct go build \
		-tags="$(PRODUCTION_BUILD_TAGS)" \
		-trimpath \
		-ldflags='-s -w -extldflags "-static"' \
		-o build/bin/$(APP_NAME) ./cmd/bnpl

.PHONY: run
# Local run always uses the development-capability binary.
run: build-development up
	@chmod +x $(ENTRYPOINT) && ./$(ENTRYPOINT)

.PHONY: init-prod
# One-time production bootstrap uses a production-capability binary.
init-prod: build-production
	@./build/bin/$(APP_NAME) db init-prod

.PHONY: init-dev
# Destructive development reset exists only in development-capability binaries.
init-dev: build-development
	@./build/bin/$(APP_NAME) db init-dev

.PHONY: policy-assignment-import-trusted
# Bulk-assign the trusted customer policy bundle from an authoritative subject CSV.
policy-assignment-import-trusted: build
	@./build/bin/$(APP_NAME) policy-assignment import-trusted \
		--file $(POLICY_ASSIGNMENT_FILE) \
		--actor $(POLICY_ASSIGNMENT_ACTOR)

# -----------------------------------------------------------------------------
# Test / Quality
# -----------------------------------------------------------------------------

# Normal/unit package tests (development build tags; no integration tag).
.PHONY: test
test:
	@$(GOTESTSUM) --format $(GOTESTSUM_FORMAT) -- -tags="$(DEVELOPMENT_BUILD_TAGS)" $(GO_TEST_PACKAGES)

# Integration tests (development + integration tags). May skip when PostgreSQL
# / infrastructure env is unavailable — treat skips as incomplete verification.
.PHONY: test-integration
test-integration:
	@DOCKER_HOST=$(DOCKER_HOST_PODMAN) $(GOTESTSUM) --format $(GOTESTSUM_FORMAT) -- \
		-tags="$(DEVELOPMENT_BUILD_TAGS) integration" -count=1 -timeout=15m -p 1 $(GO_TEST_PACKAGES)

# Runs normal tests then integration tests sequentially. Fails if either fails.
.PHONY: test-all
test-all:
	@$(MAKE) test
	@$(MAKE) test-integration

.PHONY: test-e2e
test-e2e:
	@$(GOTESTSUM) --format $(GOTESTSUM_FORMAT) -- \
		-tags="$(DEVELOPMENT_BUILD_TAGS) e2e" -count=1 -timeout=15m -p 1 ./test/e2e

.PHONY: test-coverage
test-coverage:
	@$(GOTESTSUM) --format $(GOTESTSUM_FORMAT) -- \
		-tags="$(DEVELOPMENT_BUILD_TAGS)" -coverprofile=coverage.out -covermode=atomic $(GO_TEST_PACKAGES)
	@go tool cover -func=coverage.out

.PHONY: lint
lint:
	@golangci-lint run ./...

.PHONY: staticcheck
staticcheck:
	@staticcheck -tags="$(DEVELOPMENT_BUILD_TAGS)" ./...

.PHONY: revive
revive:
	@revive -formatter friendly ./...

.PHONY: tidy-imports
tidy-imports:
	@goimports -w ./cmd ./pkg ./internal ./test ./gen ./proto

# -----------------------------------------------------------------------------
# Code Generation
# -----------------------------------------------------------------------------

.PHONY: proto
proto: mkdir-gen
	@protoc \
		--proto_path=$(PROTO_DIR) \
		--proto_path=/usr/include \
		--go_out=$(PROTO_GEN_DIR) \
		--go_opt=paths=source_relative \
		--go-grpc_out=$(PROTO_GEN_DIR) \
		--go-grpc_opt=paths=source_relative \
		$(PROTO_FILES)

.PHONY: mock
mock:
	@mockery

.PHONY: swagger
swagger:
	@mkdir -p $(SWAG_OUTPUT_DIR)
	@swag init \
		--generalInfo $(SWAG_ENTRYPOINT) \
		--dir $(SWAG_SEARCH_DIRS) \
		--output $(SWAG_OUTPUT_DIR) \
		--outputTypes go,json,yaml \
		--parseInternal \
		--packageName swagger

.PHONY: swagger-fmt
swagger-fmt:
	@swag fmt \
		--exclude gen/proto

# -----------------------------------------------------------------------------
# Docker / Local Infrastructure
# -----------------------------------------------------------------------------

.PHONY: up
up:
	@docker compose -f $(COMPOSE_FILE) up -d

.PHONY: down
down:
	@docker compose -f $(COMPOSE_FILE) down

.PHONY: build-image
build-image:
	@docker build -t $(IMAGE_NAME):$(IMAGE_VERSION) .

.PHONY: publish-image
publish-image-to-hamravesh: build-image
	@docker image tag $(HAMRAVESH_IMAGE_LOCAL_TAG) $(HAMRAVESH_IMAGE_REMOTE)
	@docker push $(HAMRAVESH_IMAGE_REMOTE)
	@echo "Published: $(HAMRAVESH_IMAGE_REMOTE)"

.PHONY: run-image
run-image:
	@docker run --rm $(IMAGE_NAME):$(IMAGE_VERSION)

.PHONY: list-images
list-images:
	@docker images | grep $(IMAGE_NAME) || true

.PHONY: clean-db
clean-db:
	@docker stop $(DB_CONTAINER_NAME) 2>/dev/null || true
	@docker rm $(DB_CONTAINER_NAME) 2>/dev/null || true
	@docker volume rm -f $(DB_DOCKER_VOLUME_NAME)
	@echo "BNPL database cleaned successfully"

# -----------------------------------------------------------------------------
# Setup / Tooling
# -----------------------------------------------------------------------------

.PHONY: install-tools
install-tools: install-golangci-lint install-staticcheck install-revive install-goimports install-mockery install-proto-tools install-swag install-gotestsum install-hooks

.PHONY: install-golangci-lint
install-golangci-lint:
	@curl -sSfL https://golangci-lint.run/install.sh | sh -s -- -b $$(go env GOPATH)/bin $(GOLANGCI_LINT_VERSION)

.PHONY: install-staticcheck
install-staticcheck:
	@go install honnef.co/go/tools/cmd/staticcheck@$(STATICCHECK_VERSION)

.PHONY: install-revive
install-revive:
	@go install github.com/mgechev/revive@latest

.PHONY: install-goimports
install-goimports:
	@go install golang.org/x/tools/cmd/goimports@latest

.PHONY: install-mockery
install-mockery:
	@go install github.com/vektra/mockery/v3@$(MOCKERY_VERSION)

.PHONY: install-proto-tools
install-proto-tools:
	@go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
	@go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest

.PHONY: install-swag
install-swag:
	@go install github.com/swaggo/swag/cmd/swag@$(SWAG_VERSION)

.PHONY: install-gotestsum
install-gotestsum:
	@go install gotest.tools/gotestsum@$(GOTESTSUM_VERSION)

.PHONY: upgrade-dependencies
upgrade-dependencies:
	@go get -v -u ./...
	@go mod tidy

.PHONY: link-migrations
link-migrations:
	@mkdir -p assets/db/init
	@ln -sf $(PWD)/pkg/adapter/db/postgres/migrationsrp/000_dev.sql assets/db/init/000_dev.sql
	@ln -sf $(PWD)/pkg/adapter/db/postgres/migrationsrp/000_prod.sql assets/db/init/000_prod.sql
	@ln -sf $(PWD)/pkg/adapter/db/postgres/migrationsrp/000_schema.sql assets/db/init/000_schema.sql
	@echo "Symbolic links created successfully"

.PHONY: install-hooks
install-hooks:
	@chmod +x .githooks/pre-commit
	@git config core.hooksPath .githooks
	@echo "Git hooks installed successfully."

# -----------------------------------------------------------------------------
# Environment / Directories / Cleanup
# -----------------------------------------------------------------------------

.PHONY: env
env:
	@grep -v '^\s*#' .env | grep -v '^\s*$$' | sed 's/^/export /'

.env: .env.example
	@if [ -f .env ]; then \
		echo ".env already exists, skipping creation."; \
	else \
		# Drop both comments and blank lines when materializing .env.example. \
		grep -v '^#' .env.example | grep -v '^[[:space:]]*$$' > .env; \
		echo ".env created from .env.example"; \
	fi

doc/collections/http-client.env.json: doc/collections/http-client.env.json.example
	@if [ -f doc/collections/http-client.env.json ]; then \
		echo "doc/collections/http-client.env.json already exists, skipping creation."; \
	else \
		cp doc/collections/http-client.env.json.example doc/collections/http-client.env.json; \
		echo "doc/collections/http-client.env.json created from example."; \
	fi

.PHONY: http-client-env
http-client-env: doc/collections/http-client.env.json

.PHONY: mkdir
mkdir: mkdir-bin mkdir-gen

.PHONY: mkdir-bin
mkdir-bin:
	@mkdir -p build/bin

.PHONY: mkdir-gen
mkdir-gen:
	@mkdir -p $(PROTO_GEN_DIR)

.PHONY: list-interfaces
list-interfaces:
	@path=$(if $(ARGS),$(ARGS),pkg/core); \
	if [ ! -d "$$path" ]; then \
	   echo "❌ Error: Directory '$$path' does not exist"; \
	   exit 1; \
	fi; \
	echo "🔍 Scanning interfaces in $$path..."; \
	echo; \
	found=0; \
	for dir in "$$path" "$$path"/*/; do \
	   if [ -d "$$dir" ]; then \
	      name=$$(basename "$$dir"); \
	      interfaces=$$(grep -rn "type.*interface {" "$$dir" --include="*.go" 2>/dev/null | awk '{print $$2}' | sort -u); \
	      if [ ! -z "$$interfaces" ]; then \
	         found=1; \
	         count=$$(echo "$$interfaces" | wc -l); \
	         echo "📂 $$name/ ($$count interfaces)"; \
	         echo "$$interfaces" | while read iface; do \
	            echo "   ├── $$iface"; \
	         done; \
	         echo; \
	      fi; \
	   fi; \
	done; \
	if [ $$found -eq 0 ]; then \
	   echo "ℹ️  No interfaces found in $$path"; \
	fi

.PHONY: clean
clean:
	@rm -f ./build/bin/$(APP_NAME) coverage.out

CONFIG ?= configs/config.yaml

.PHONY: db-invariants-list db-invariants-verify db-invariants-test
db-invariants-list:
	@go run -tags development ./cmd/bnpl db invariants list

db-invariants-verify:
	@go run -tags development ./cmd/bnpl --config "$(CONFIG)" db invariants verify

db-invariants-test:
	@go test -count=1 -race -tags 'development integration' ./pkg/adapter/db/postgres/invariants/...
