APP_NAME := "bnpl"
IMAGE_NAME ?= "bnpl_image"
IMAGE_VERSION ?= "0.0.1"
ENTRYPOINT ?= scripts/docker-entrypoint.sh

.PHINY: all
all: build

.PHONY: mkdir build
build:
	CGO_ENABLED=0 go build -tags="go_json nomsgpack" -trimpath -ldflags '-s -w -extldflags "-static"' \
	-o build/bin/${APP_NAME} ./cmd/bnpl

.PHONY: run
run: build
	@chmod +x $(ENTRYPOINT) && ./$(ENTRYPOINT)

.PHONY: test
test:
	@go test ./...

.PHONY: install-staticcheck install-revive revive lint
install-staticcheck:
	go install honnef.co/go/tools/cmd/staticcheck@2025.1.1

install-revive:
	go get -u github.com/mgechev/revive

revive:
	revive -formatter friendly ./...

lint:
	@staticcheck ./...
	@revive ./...

.PHONY: install-tidy-imports
install-tidy-imports:
	@go install golang.org/x/tools/cmd/goimports@latest

.PHONY: tidy-imports
tidy-imports:
	@goimports -w .

.PHONY: up
up:
	@docker compose -f docker-compose.yml up -d

.PHONY: build-image
build-image:
	@docker build -t $(IMAGE_NAME):$(IMAGE_VERSION) .

.PHONY: list-images
list-images:
	@docker images | grep $(IMAGE_NAME)

.PHONY: run-image
run-image:
	@docker run --rm $(IMAGE_NAME):$(IMAGE_VERSION)

.PHONY: clean
clean:
	@rm -f ./build/bin/$(APP_NAME)

.PHONY: mkdir
mkdir:
	@mkdir -p build/bin
