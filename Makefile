# Auto-load .env so every Make target sees the variables.
-include .env
export

.DEFAULT_GOAL := build

APP_NAME := bnpl
IMAGE_NAME ?= bnpl-backend
IMAGE_VERSION ?= v0.1.0
ENTRYPOINT ?= scripts/docker-entrypoint.sh

HAMRAVESH_CONTAINER_REGISTRY ?= registry.hamdocker.ir
HAMRAVESH_REGISTRY_NAMESPACE ?= parstis-technology

HAMRAVESH_IMAGE_LOCAL_TAG := $(IMAGE_NAME):$(IMAGE_VERSION)
HAMRAVESH_IMAGE_REMOTE := $(HAMRAVESH_CONTAINER_REGISTRY)/$(HAMRAVESH_REGISTRY_NAMESPACE)/$(IMAGE_NAME):$(IMAGE_VERSION)

DB_CONTAINER_NAME ?= bnpl-db
DB_DOCKER_VOLUME_NAME ?= bnpl-backend_postgres_data_bnpl

PROTO_DIR := proto
PROTO_GEN_DIR := gen/proto
PROTO_FILES := $(shell find $(PROTO_DIR) -name '*.proto')

GOLANGCI_LINT_VERSION ?= v2.12.2
MOCKERY_VERSION ?= v3.7.1
STATICCHECK_VERSION ?= 2025.1.1

COMPOSE_FILE ?= docker-compose.local.yml
DOCKER_HOST_PODMAN ?= unix://$(XDG_RUNTIME_DIR)/podman/podman.sock

.PHONY: all
all: build

# -----------------------------------------------------------------------------
# Build / Run
# -----------------------------------------------------------------------------

.PHONY: build
build: mkdir-bin
	CGO_ENABLED=0 GOPROXY=https://goproxy.io,direct go build \
		-tags="go_json nomsgpack" \
		-trimpath \
		-ldflags='-s -w -extldflags "-static"' \
		-o build/bin/$(APP_NAME) ./cmd/bnpl

.PHONY: run
run: build up
	@chmod +x $(ENTRYPOINT) && ./$(ENTRYPOINT)

.PHONY: init-prod
init-prod: build
	@./build/bin/$(APP_NAME) db init-prod

.PHONY: init-dev
init-dev: build
	@./build/bin/$(APP_NAME) db init-dev

# -----------------------------------------------------------------------------
# Test / Quality
# -----------------------------------------------------------------------------

.PHONY: test
test:
	@go test ./...

.PHONY: test-integration
test-integration:
	@DOCKER_HOST=$(DOCKER_HOST_PODMAN) go test -tags integration -count=1 -timeout=15m -p 1 ./...

.PHONY: test-coverage
test-coverage:
	@go test ./... -coverprofile=coverage.out -covermode=atomic
	@go tool cover -func=coverage.out

.PHONY: lint
lint:
	@golangci-lint run ./...

.PHONY: staticcheck
staticcheck:
	@staticcheck ./...

.PHONY: revive
revive:
	@revive -formatter friendly ./...

.PHONY: tidy-imports
tidy-imports:
	@goimports -w .

# -----------------------------------------------------------------------------
# Code Generation
# -----------------------------------------------------------------------------

.PHONY: proto
proto: mkdir-gen
	@protoc \
		--proto_path=$(PROTO_DIR) \
		--go_out=$(PROTO_GEN_DIR) \
		--go_opt=paths=source_relative \
		--go-grpc_out=$(PROTO_GEN_DIR) \
		--go-grpc_opt=paths=source_relative \
		$(PROTO_FILES)

.PHONY: mock
mock:
	@mockery

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
install-tools: install-golangci-lint install-staticcheck install-revive install-goimports install-mockery install-proto-tools

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

.PHONY: upgrade-dependencies
upgrade-dependencies:
	@go get -v -u ./...
	@go mod tidy

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
