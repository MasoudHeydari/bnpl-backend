# Build / publish the dpay Envoy gateway image (config + TLS baked in).
#
#   make image
#   make image TAG=v0.1.1
#   make push TAG=v0.1.0
#   make push-to-hamravesh TAG=v0.1.0

TAG ?= v0.1.0
IMAGE_NAME ?= dpay-envoy
IMAGE_REPO ?= registry.hamdocker.ir/parstis-technology/dpay-envoy
LOCAL_IMAGE ?= $(IMAGE_NAME):$(TAG)
REMOTE_IMAGE ?= $(IMAGE_REPO):$(TAG)

.PHONY: image push push-to-hamravesh

image:
	@test -f certs/tls.crt || (echo "missing certs/tls.crt" >&2; exit 1)
	@test -f certs/tls.key || (echo "missing certs/tls.key" >&2; exit 1)
	docker build -t "$(LOCAL_IMAGE)" -f Dockerfile .
	docker tag "$(LOCAL_IMAGE)" "$(REMOTE_IMAGE)"

# Docker Engine push of the local tag (dpay-envoy:$(TAG)).
push: image
	docker push "$(LOCAL_IMAGE)"

# Push to Hamravesh registry (registry.hamdocker.ir/.../dpay-envoy:$(TAG)).
push-to-hamravesh: image
	docker push "$(REMOTE_IMAGE)"
