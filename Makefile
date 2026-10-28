# Auto-load an existing .env. Commands do not create or modify it implicitly.
ifneq ($(wildcard .env),)
include .env
endif
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
REVIVE_VERSION ?= v1.17.1
GOIMPORTS_VERSION ?= v0.44.0
PROTOC_GEN_GO_VERSION ?= v1.36.11
PROTOC_GEN_GO_GRPC_VERSION ?= v1.6.2
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
		-o build/bin/$(APP_NAME)-development ./cmd/bnpl
	@cp build/bin/$(APP_NAME)-development build/bin/$(APP_NAME)

.PHONY: build-production
build-production: mkdir-bin
	CGO_ENABLED=0 GOPROXY=https://goproxy.io,direct go build \
		-tags="$(PRODUCTION_BUILD_TAGS)" \
		-trimpath \
		-ldflags='-s -w -extldflags "-static"' \
		-o build/bin/$(APP_NAME)-production ./cmd/bnpl

.PHONY: run
# Local run always uses the development-capability binary.
run: build-development up
	@chmod +x $(ENTRYPOINT) && CONFIG_FILE="$(RUNTIME_CONFIG)" ./$(ENTRYPOINT)

# Compatibility aliases now invoke the separate database executable.
.PHONY: init-prod init-dev
init-prod: db-init-production
init-dev: db-init-development

.PHONY: policy-assignment-import-trusted
# Bulk-assign the trusted customer policy bundle from an authoritative subject CSV.
policy-assignment-import-trusted: build
	@./build/bin/$(APP_NAME) --config "$(RUNTIME_CONFIG)" policy-assignment import-trusted \
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

# Legacy image targets remain Production Runtime only.
.PHONY: build-image publish-image publish-image-to-hamravesh
build-image: build-image-production
	@docker tag $(IMAGE_NAME):$(IMAGE_VERSION)-production $(HAMRAVESH_IMAGE_LOCAL_TAG)
publish-image-to-hamravesh: build-image
	@docker tag $(HAMRAVESH_IMAGE_LOCAL_TAG) $(HAMRAVESH_IMAGE_REMOTE)
	@docker push $(HAMRAVESH_IMAGE_REMOTE)
publish-image: publish-image-to-hamravesh

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
install-tools: install-golangci-lint install-staticcheck install-revive install-goimports install-mockery install-proto-tools install-swag install-gotestsum install-hooks install-shfmt

.PHONY: install-golangci-lint
install-golangci-lint:
	@curl -sSfL https://golangci-lint.run/install.sh | sh -s -- -b $$(go env GOPATH)/bin $(GOLANGCI_LINT_VERSION)

.PHONY: install-staticcheck
install-staticcheck:
	@GOTOOLCHAIN=local go install honnef.co/go/tools/cmd/staticcheck@$(STATICCHECK_VERSION)

.PHONY: install-revive
install-revive:
	@GOTOOLCHAIN=local go install github.com/mgechev/revive@$(REVIVE_VERSION)

.PHONY: install-goimports
install-goimports:
	@GOTOOLCHAIN=local go install golang.org/x/tools/cmd/goimports@$(GOIMPORTS_VERSION)

.PHONY: install-mockery
install-mockery:
	@GOTOOLCHAIN=local go install github.com/vektra/mockery/v3@$(MOCKERY_VERSION)

.PHONY: install-proto-tools
install-proto-tools:
	@GOTOOLCHAIN=local go install google.golang.org/protobuf/cmd/protoc-gen-go@$(PROTOC_GEN_GO_VERSION)
	@GOTOOLCHAIN=local go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@$(PROTOC_GEN_GO_GRPC_VERSION)

.PHONY: install-swag
install-swag:
	@GOTOOLCHAIN=local go install github.com/swaggo/swag/cmd/swag@$(SWAG_VERSION)

.PHONY: install-gotestsum
install-gotestsum:
	@GOTOOLCHAIN=local go install gotest.tools/gotestsum@$(GOTESTSUM_VERSION)

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
	@if [ "$$(git config --local --get core.hooksPath || true)" = .githooks ] && [ -x .githooks/pre-commit ]; then \
		echo "Git hooks already configured."; \
	else \
		chmod +x .githooks/pre-commit; \
		if [ "$$(git config --local --get core.hooksPath || true)" != .githooks ]; then git config --local core.hooksPath .githooks; fi; \
		echo "Git hooks installed successfully."; \
	fi

.PHONY: install-shfmt
install-shfmt:
	@go install mvdan.cc/sh/v3/cmd/shfmt@latest

# -----------------------------------------------------------------------------
# Environment / Directories / Cleanup
# -----------------------------------------------------------------------------

.PHONY: env
env:
	@sed -n 's/^\([A-Za-z_][A-Za-z0-9_]*\)=.*/\1=<redacted>/p' .env

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
	@rm -f ./build/bin/$(APP_NAME) ./build/bin/$(APP_NAME)-development ./build/bin/$(APP_NAME)-production ./build/bin/bnpl-dbadmin-development ./build/bin/bnpl-dbadmin-production coverage.out

# Runtime and DB Admin configs are separate; CLI --config can override them.
CONFIG ?= configs/dbadmin.yaml
RUNTIME_CONFIG ?= configs/runtime.yaml
DBADMIN_IMAGE_NAME ?= $(IMAGE_NAME)-dbadmin
PG_FORMAT ?= pg_format

# -----------------------------------------------------------------------------
# DB Admin binaries
# -----------------------------------------------------------------------------

.PHONY: build-dbadmin-development
build-dbadmin-development: mkdir-bin
	CGO_ENABLED=0 go build -tags="$(DEVELOPMENT_BUILD_TAGS)" -trimpath \
		-ldflags='-s -w -extldflags "-static"' \
		-o build/bin/bnpl-dbadmin-development ./cmd/bnpl-dbadmin

.PHONY: build-dbadmin-production
build-dbadmin-production: mkdir-bin
	CGO_ENABLED=0 go build -tags="$(PRODUCTION_BUILD_TAGS)" -trimpath \
		-ldflags='-s -w -extldflags "-static"' \
		-o build/bin/bnpl-dbadmin-production ./cmd/bnpl-dbadmin

# -----------------------------------------------------------------------------
# Database lifecycle (the Runtime executable never performs DDL)
# -----------------------------------------------------------------------------

.PHONY: db-init-development
db-init-development: build-dbadmin-development
	@./build/bin/bnpl-dbadmin-development --config "$(CONFIG)" db init-dev

.PHONY: db-init-production
db-init-production: build-dbadmin-production
	@./build/bin/bnpl-dbadmin-production --config "$(CONFIG)" db init-prod

.PHONY: db-migrate-development
db-migrate-development: build-dbadmin-development
	@./build/bin/bnpl-dbadmin-development --config "$(CONFIG)" db migrate

.PHONY: db-migrate-production
db-migrate-production: build-dbadmin-production
	@./build/bin/bnpl-dbadmin-production --config "$(CONFIG)" db migrate

.PHONY: db-invariants-list db-privileges-list
db-invariants-list:
	@go run -tags="$(DEVELOPMENT_BUILD_TAGS)" ./cmd/bnpl-dbadmin db invariants list

db-privileges-list:
	@go run -tags="$(DEVELOPMENT_BUILD_TAGS)" ./cmd/bnpl-dbadmin db privileges list

.PHONY: db-invariants-verify-development
db-invariants-verify-development: build-dbadmin-development
	@./build/bin/bnpl-dbadmin-development --config "$(CONFIG)" db invariants verify

.PHONY: db-invariants-verify-production
db-invariants-verify-production: build-dbadmin-production
	@./build/bin/bnpl-dbadmin-production --config "$(CONFIG)" db invariants verify

.PHONY: db-privileges-verify-development
db-privileges-verify-development: build-dbadmin-development
	@./build/bin/bnpl-dbadmin-development --config "$(CONFIG)" db privileges verify

.PHONY: db-privileges-verify-production
db-privileges-verify-production: build-dbadmin-production
	@./build/bin/bnpl-dbadmin-production --config "$(CONFIG)" db privileges verify

.PHONY: db-invariants-verify db-invariants-test
db-invariants-verify: db-invariants-verify-development
db-invariants-test:
	@go test -count=1 -race -tags '$(DEVELOPMENT_BUILD_TAGS) integration' ./pkg/adapter/db/postgres/invariants/...

# -----------------------------------------------------------------------------
# Docker images (building never publishes)
# -----------------------------------------------------------------------------

.PHONY: build-image-development
build-image-development:
	@docker build --target runtime --build-arg BUILD_ENV=development \
		-t $(IMAGE_NAME):$(IMAGE_VERSION)-development .

.PHONY: build-image-production
build-image-production:
	@docker build --target runtime --build-arg BUILD_ENV=production \
		-t $(IMAGE_NAME):$(IMAGE_VERSION)-production .

.PHONY: build-dbadmin-image-development
build-dbadmin-image-development:
	@docker build --target dbadmin --build-arg BUILD_ENV=development \
		-t $(DBADMIN_IMAGE_NAME):$(IMAGE_VERSION)-development .

.PHONY: build-dbadmin-image-production
build-dbadmin-image-production:
	@docker build --target dbadmin --build-arg BUILD_ENV=production \
		-t $(DBADMIN_IMAGE_NAME):$(IMAGE_VERSION)-production .

.PHONY: publish-image-development
publish-image-development: build-image-development
	@docker tag $(IMAGE_NAME):$(IMAGE_VERSION)-development \
		$(HAMRAVESH_CONTAINER_REGISTRY)/$(HAMRAVESH_REGISTRY_NAMESPACE)/$(IMAGE_NAME):$(IMAGE_VERSION)-development
	@docker push $(HAMRAVESH_CONTAINER_REGISTRY)/$(HAMRAVESH_REGISTRY_NAMESPACE)/$(IMAGE_NAME):$(IMAGE_VERSION)-development

.PHONY: publish-image-production
publish-image-production: build-image-production
	@docker tag $(IMAGE_NAME):$(IMAGE_VERSION)-production \
		$(HAMRAVESH_CONTAINER_REGISTRY)/$(HAMRAVESH_REGISTRY_NAMESPACE)/$(IMAGE_NAME):$(IMAGE_VERSION)-production
	@docker push $(HAMRAVESH_CONTAINER_REGISTRY)/$(HAMRAVESH_REGISTRY_NAMESPACE)/$(IMAGE_NAME):$(IMAGE_VERSION)-production

.PHONY: publish-dbadmin-image-development
publish-dbadmin-image-development: build-dbadmin-image-development
	@docker tag $(DBADMIN_IMAGE_NAME):$(IMAGE_VERSION)-development \
		$(HAMRAVESH_CONTAINER_REGISTRY)/$(HAMRAVESH_REGISTRY_NAMESPACE)/$(DBADMIN_IMAGE_NAME):$(IMAGE_VERSION)-development
	@docker push $(HAMRAVESH_CONTAINER_REGISTRY)/$(HAMRAVESH_REGISTRY_NAMESPACE)/$(DBADMIN_IMAGE_NAME):$(IMAGE_VERSION)-development

.PHONY: publish-dbadmin-image-production
publish-dbadmin-image-production: build-dbadmin-image-production
	@docker tag $(DBADMIN_IMAGE_NAME):$(IMAGE_VERSION)-production \
		$(HAMRAVESH_CONTAINER_REGISTRY)/$(HAMRAVESH_REGISTRY_NAMESPACE)/$(DBADMIN_IMAGE_NAME):$(IMAGE_VERSION)-production
	@docker push $(HAMRAVESH_CONTAINER_REGISTRY)/$(HAMRAVESH_REGISTRY_NAMESPACE)/$(DBADMIN_IMAGE_NAME):$(IMAGE_VERSION)-production

# -----------------------------------------------------------------------------
# Verification of the two executable/image security boundaries
# -----------------------------------------------------------------------------

.PHONY: verify-entrypoint-boundaries
verify-entrypoint-boundaries: build-development build-production build-dbadmin-development build-dbadmin-production
	@sh scripts/verify-entrypoint-boundaries.sh

.PHONY: verify-docker-boundaries
verify-docker-boundaries:
	@sh scripts/verify-docker-boundaries.sh

# -----------------------------------------------------------------------------
# SQL formatting: pgFormatter is an external developer tool (see README).
# SQL=file.sql formats one file; SQL omitted formats all regular *.sql files.
# -----------------------------------------------------------------------------

SQL ?=

.PHONY: sql-fmt
sql-fmt:
	@command -v $(PG_FORMAT) >/dev/null || { echo "Install pgFormatter first (see README.md)" >&2; exit 1; }
	@if [ -n "$(SQL)" ]; then \
		test -f "$(SQL)" || { echo "SQL file not found: $(SQL)" >&2; exit 1; }; \
		$(PG_FORMAT) --no-rcfile --config .pg_format --inplace "$(SQL)"; \
	else \
		find . -type f -name '*.sql' \
			-not -path './.git/*' -not -path './vendor/*' -not -path './build/*' \
			-print0 | xargs -0 -r -n 1 $(PG_FORMAT) --no-rcfile --config .pg_format --inplace; \
	fi

.PHONY: sql-fmt-check
sql-fmt-check:
	@command -v $(PG_FORMAT) >/dev/null || { echo "Install pgFormatter first (see README.md)" >&2; exit 1; }
	@if [ -n "$(SQL)" ]; then \
		test -f "$(SQL)" || { echo "SQL file not found: $(SQL)" >&2; exit 1; }; \
		$(PG_FORMAT) --no-rcfile --config .pg_format "$(SQL)" | diff -u "$(SQL)" -; \
	else \
		find . -type f -name '*.sql' \
			-not -path './.git/*' -not -path './vendor/*' -not -path './build/*' \
			-print0 | xargs -0 -r -n 1 sh -c '$(PG_FORMAT) --no-rcfile --config .pg_format "$$0" | diff -u "$$0" -'; \
	fi

# -----------------------------------------------------------------------------
# Shell formatting: shfmt is an external developer tool (see README).
# It formats all *.sh files in ./scripts directory.
# -----------------------------------------------------------------------------

.PHONY: shell-fmt
shell-fmt:
	@shfmt -l -w ./scripts
