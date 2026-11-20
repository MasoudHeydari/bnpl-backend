# Build / push BNPL runtime and dbadmin images from the monorepo root Dockerfile.
#
#   make image
#   make image-dbadmin
#   make push-to-hamravesh
#   make push-dbadmin-to-hamravesh
#   make image BUILD_ENV=production TAG=v0.1.0-production
#
# image* and push* are separate: push never rebuilds.

REPO_ROOT := $(abspath ../../..)

BUILD_ENV ?= development
TAG ?= v0.1.0-$(BUILD_ENV)
IMAGE_NAME ?= bnpl-backend
DBADMIN_IMAGE_NAME ?= bnpl-backend-dbadmin
REGISTRY ?= registry.hamdocker.ir
REGISTRY_NAMESPACE ?= parstis-technology

LOCAL_IMAGE ?= $(IMAGE_NAME):$(TAG)
REMOTE_IMAGE ?= $(REGISTRY)/$(REGISTRY_NAMESPACE)/$(IMAGE_NAME):$(TAG)
LOCAL_DBADMIN_IMAGE ?= $(DBADMIN_IMAGE_NAME):$(TAG)
REMOTE_DBADMIN_IMAGE ?= $(REGISTRY)/$(REGISTRY_NAMESPACE)/$(DBADMIN_IMAGE_NAME):$(TAG)

DOCKERFILE ?= $(REPO_ROOT)/Dockerfile

.PHONY: help \
	image tag push push-to-hamravesh \
	image-dbadmin tag-dbadmin push-dbadmin push-dbadmin-to-hamravesh \
	print-image

help:
	@printf '%s\n' \
		'Runtime:' \
		'  make image                 Build runtime (BUILD_ENV=$(BUILD_ENV))' \
		'  make tag                   Retag runtime local -> remote' \
		'  make push                  Push LOCAL_IMAGE only' \
		'  make push-to-hamravesh     Push REMOTE_IMAGE only' \
		'' \
		'DB admin:' \
		'  make image-dbadmin' \
		'  make tag-dbadmin' \
		'  make push-dbadmin' \
		'  make push-dbadmin-to-hamravesh' \
		'' \
		'Override: BUILD_ENV TAG IMAGE_NAME DBADMIN_IMAGE_NAME REGISTRY REGISTRY_NAMESPACE' \
		'Example: make image BUILD_ENV=production TAG=v0.1.0-production'

print-image:
	@printf 'LOCAL_IMAGE=%s\nREMOTE_IMAGE=%s\nLOCAL_DBADMIN_IMAGE=%s\nREMOTE_DBADMIN_IMAGE=%s\n' \
		"$(LOCAL_IMAGE)" "$(REMOTE_IMAGE)" "$(LOCAL_DBADMIN_IMAGE)" "$(REMOTE_DBADMIN_IMAGE)"

# development → runtime-development (bakes deploy/bnpl/hamravesh/certs)
# production  → runtime (no client PEMs in image layers)
RUNTIME_TARGET = $(if $(filter development,$(BUILD_ENV)),runtime-development,runtime)

image:
	@case "$(BUILD_ENV)" in development|production) ;; *) \
		echo "BUILD_ENV must be development or production (got $(BUILD_ENV))" >&2; exit 1;; esac
	@test -f "$(REPO_ROOT)/deploy/bnpl/hamravesh/certs/ca.crt"
	@test -f "$(REPO_ROOT)/deploy/bnpl/hamravesh/certs/bnpl-service.crt"
	@if [ "$(BUILD_ENV)" = "development" ]; then \
		test -f "$(REPO_ROOT)/deploy/bnpl/hamravesh/certs/bnpl-service.key" || { \
			echo "missing deploy/bnpl/hamravesh/certs/bnpl-service.key for development image" >&2; \
			exit 1; \
		}; \
	fi
	docker build \
		--target "$(RUNTIME_TARGET)" \
		--build-arg BUILD_ENV="$(BUILD_ENV)" \
		-t "$(LOCAL_IMAGE)" \
		-f "$(DOCKERFILE)" \
		"$(REPO_ROOT)"
	docker tag "$(LOCAL_IMAGE)" "$(REMOTE_IMAGE)"

tag:
	docker tag "$(LOCAL_IMAGE)" "$(REMOTE_IMAGE)"

push:
	docker push "$(LOCAL_IMAGE)"

push-to-hamravesh:
	docker push "$(REMOTE_IMAGE)"

image-dbadmin:
	@case "$(BUILD_ENV)" in development|production) ;; *) \
		echo "BUILD_ENV must be development or production (got $(BUILD_ENV))" >&2; exit 1;; esac
	docker build \
		--target dbadmin \
		--build-arg BUILD_ENV="$(BUILD_ENV)" \
		-t "$(LOCAL_DBADMIN_IMAGE)" \
		-f "$(DOCKERFILE)" \
		"$(REPO_ROOT)"
	docker tag "$(LOCAL_DBADMIN_IMAGE)" "$(REMOTE_DBADMIN_IMAGE)"

tag-dbadmin:
	docker tag "$(LOCAL_DBADMIN_IMAGE)" "$(REMOTE_DBADMIN_IMAGE)"

push-dbadmin:
	docker push "$(LOCAL_DBADMIN_IMAGE)"

push-dbadmin-to-hamravesh:
	docker push "$(REMOTE_DBADMIN_IMAGE)"
