# Build / publish the dpay Postgres image (multi-DB init script baked in).
#
#   make image
#   make image TAG=v0.1.1
#   make push TAG=v0.1.0
#   make push-to-hamravesh TAG=v0.1.0

TAG ?= v0.1.0
IMAGE_NAME ?= dpay-postgres
IMAGE_REPO ?= registry.hamdocker.ir/parstis-technology/dpay-postgres
LOCAL_IMAGE ?= $(IMAGE_NAME):$(TAG)
REMOTE_IMAGE ?= $(IMAGE_REPO):$(TAG)

.PHONY: image push push-to-hamravesh

image:
	@test -f init-databases.sh || (echo "missing init-databases.sh" >&2; exit 1)
	docker build -t "$(LOCAL_IMAGE)" -f Dockerfile .
	docker tag "$(LOCAL_IMAGE)" "$(REMOTE_IMAGE)"

# Docker Engine push of the local tag (dpay-postgres:$(TAG)).
push: image
	docker push "$(LOCAL_IMAGE)"

# Push to Hamravesh registry (registry.hamdocker.ir/.../dpay-postgres:$(TAG)).
push-to-hamravesh: image
	docker push "$(REMOTE_IMAGE)"
