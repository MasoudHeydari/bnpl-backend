# Customer service image helpers for Hamravesh.
#
# This repository does not own the Customer Dockerfile. Use `tag` to retag an
# already-built local image, then `push-to-hamravesh`.
#
#   make tag SOURCE_IMAGE=customer-service:local
#   make push-to-hamravesh

TAG ?= v0.1.0
IMAGE_NAME ?= customer-service
REGISTRY ?= registry.hamdocker.ir
REGISTRY_NAMESPACE ?= parstis-technology
IMAGE_REPO ?= $(REGISTRY)/$(REGISTRY_NAMESPACE)/$(IMAGE_NAME)
LOCAL_IMAGE ?= $(IMAGE_NAME):$(TAG)
REMOTE_IMAGE ?= $(IMAGE_REPO):$(TAG)
SOURCE_IMAGE ?=

.PHONY: help image tag push push-to-hamravesh print-image

help:
	@printf '%s\n' \
		'Targets:' \
		'  make image              Not supported here (no Dockerfile in this repo)' \
		'  make tag SOURCE_IMAGE=  Retag SOURCE_IMAGE (or LOCAL_IMAGE) -> REMOTE_IMAGE' \
		'  make push               Push LOCAL_IMAGE only' \
		'  make push-to-hamravesh  Push REMOTE_IMAGE only' \
		'' \
		'Override: TAG IMAGE_NAME REGISTRY REGISTRY_NAMESPACE IMAGE_REPO SOURCE_IMAGE'

print-image:
	@printf 'LOCAL_IMAGE=%s\nREMOTE_IMAGE=%s\nSOURCE_IMAGE=%s\n' \
		"$(LOCAL_IMAGE)" "$(REMOTE_IMAGE)" "$(SOURCE_IMAGE)"

image:
	@echo "customer image is not built from this repository; use tag + push-to-hamravesh" >&2
	@exit 1

tag:
	@src="$(SOURCE_IMAGE)"; \
	if [ -z "$$src" ]; then src="$(LOCAL_IMAGE)"; fi; \
	docker tag "$$src" "$(REMOTE_IMAGE)"

push:
	docker push "$(LOCAL_IMAGE)"

push-to-hamravesh:
	docker push "$(REMOTE_IMAGE)"
