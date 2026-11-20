# Gateway image (Envoy). Build context is ./envoy.
#
#   make image
#   make push-to-hamravesh
#   make image TAG=v0.1.1

.PHONY: help image tag push push-to-hamravesh print-image

help:
	@$(MAKE) -C envoy help

image:
	$(MAKE) -C envoy image

tag:
	$(MAKE) -C envoy tag

push:
	$(MAKE) -C envoy push

push-to-hamravesh:
	$(MAKE) -C envoy push-to-hamravesh

print-image:
	$(MAKE) -C envoy print-image
