# Infrastructure images owned by this repo (postgres + kafka-init).
# Separate targets only — nothing builds/pushes everything in one shot.
#
#   make postgres-image
#   make postgres-push-to-hamravesh
#   make kafka-init-image
#   make kafka-init-push-to-hamravesh

.PHONY: help \
	postgres-image postgres-tag postgres-push postgres-push-to-hamravesh \
	kafka-init-image kafka-init-tag kafka-init-push kafka-init-push-to-hamravesh

help:
	@printf '%s\n' \
		'postgres-* and kafka-init-* delegate to subdirectory Makefiles.' \
		'Pass TAG=... IMAGE_NAME=... through as needed.' \
		'' \
		'  make postgres-image' \
		'  make postgres-push-to-hamravesh' \
		'  make kafka-init-image' \
		'  make kafka-init-push-to-hamravesh'

postgres-image:
	$(MAKE) -C postgres image

postgres-tag:
	$(MAKE) -C postgres tag

postgres-push:
	$(MAKE) -C postgres push

postgres-push-to-hamravesh:
	$(MAKE) -C postgres push-to-hamravesh

kafka-init-image:
	$(MAKE) -C kafka-init image

kafka-init-tag:
	$(MAKE) -C kafka-init tag

kafka-init-push:
	$(MAKE) -C kafka-init push

kafka-init-push-to-hamravesh:
	$(MAKE) -C kafka-init push-to-hamravesh
