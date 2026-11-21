# Infrastructure images owned by this repo.
# Separate targets only — nothing builds/pushes everything in one shot.
#
#   make postgres-image
#   make postgres-push-to-hamravesh
#   make kafka-init-image
#   make kafka-init-push-to-hamravesh
#   make loki-image / loki-push-to-hamravesh
#   make promtail-image / promtail-push-to-hamravesh
#   make grafana-image / grafana-push-to-hamravesh

.PHONY: help \
	postgres-image postgres-tag postgres-push postgres-push-to-hamravesh \
	kafka-init-image kafka-init-tag kafka-init-push kafka-init-push-to-hamravesh \
	loki-image loki-tag loki-push loki-push-to-hamravesh \
	promtail-image promtail-tag promtail-push promtail-push-to-hamravesh \
	grafana-image grafana-tag grafana-push grafana-push-to-hamravesh

help:
	@printf '%s\n' \
		'postgres-*, kafka-init-*, loki-*, promtail-*, grafana-* delegate to subdirectory Makefiles.' \
		'Pass TAG=... IMAGE_NAME=... through as needed.' \
		'' \
		'  make postgres-image && make postgres-push-to-hamravesh' \
		'  make kafka-init-image && make kafka-init-push-to-hamravesh' \
		'  make loki-image && make loki-push-to-hamravesh' \
		'  make promtail-image && make promtail-push-to-hamravesh' \
		'  make grafana-image && make grafana-push-to-hamravesh'

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

loki-image:
	$(MAKE) -C loki image

loki-tag:
	$(MAKE) -C loki tag

loki-push:
	$(MAKE) -C loki push

loki-push-to-hamravesh:
	$(MAKE) -C loki push-to-hamravesh

promtail-image:
	$(MAKE) -C promtail image

promtail-tag:
	$(MAKE) -C promtail tag

promtail-push:
	$(MAKE) -C promtail push

promtail-push-to-hamravesh:
	$(MAKE) -C promtail push-to-hamravesh

grafana-image:
	$(MAKE) -C grafana image

grafana-tag:
	$(MAKE) -C grafana tag

grafana-push:
	$(MAKE) -C grafana push

grafana-push-to-hamravesh:
	$(MAKE) -C grafana push-to-hamravesh
