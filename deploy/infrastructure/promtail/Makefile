# Build / push the dpay Promtail image (config baked in).
#
#   make image
#   make image TAG=v0.1.1
#   make push-to-hamravesh TAG=v0.1.0
#
# image and push are separate: push never rebuilds.

TAG ?= v0.1.0
IMAGE_NAME ?= dpay-promtail
REGISTRY ?= registry.hamdocker.ir
REGISTRY_NAMESPACE ?= parstis-technology
IMAGE_REPO ?= $(REGISTRY)/$(REGISTRY_NAMESPACE)/$(IMAGE_NAME)
LOCAL_IMAGE ?= $(IMAGE_NAME):$(TAG)
REMOTE_IMAGE ?= $(IMAGE_REPO):$(TAG)

.PHONY: help image tag push push-to-hamravesh print-image

help:
	@printf '%s\n' \
		'Targets:' \
		'  make image              Build local + tag remote name' \
		'  make tag                Retag LOCAL_IMAGE -> REMOTE_IMAGE' \
		'  make push               Push LOCAL_IMAGE only (no rebuild)' \
		'  make push-to-hamravesh  Push REMOTE_IMAGE only (no rebuild)' \
		'  make print-image        Show resolved image names' \
		'' \
		'Override: TAG IMAGE_NAME REGISTRY REGISTRY_NAMESPACE IMAGE_REPO'

print-image:
	@printf 'LOCAL_IMAGE=%s\nREMOTE_IMAGE=%s\n' "$(LOCAL_IMAGE)" "$(REMOTE_IMAGE)"

image:
	@test -f config.yml || (echo "missing config.yml" >&2; exit 1)
	@test -f entrypoint.sh || (echo "missing entrypoint.sh" >&2; exit 1)
	docker build -t "$(LOCAL_IMAGE)" -f Dockerfile .
	docker tag "$(LOCAL_IMAGE)" "$(REMOTE_IMAGE)"

tag:
	docker tag "$(LOCAL_IMAGE)" "$(REMOTE_IMAGE)"

push:
	docker push "$(LOCAL_IMAGE)"

push-to-hamravesh:
	docker push "$(REMOTE_IMAGE)"
