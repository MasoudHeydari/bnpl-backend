# Index of per-app image build/push Makefiles under deploy/.
# No all-in-one publish target — each app/command stays separate.
#
# Built from this repo:
#   make -C bnpl/hamravesh image
#   make -C bnpl/hamravesh push-to-hamravesh
#   make -C bnpl/hamravesh image-dbadmin
#   make -C bnpl/hamravesh push-dbadmin-to-hamravesh
#   make -C gateway image
#   make -C gateway push-to-hamravesh
#   make -C infrastructure postgres-image
#   make -C infrastructure postgres-push-to-hamravesh
#   make -C infrastructure kafka-init-image
#   make -C infrastructure kafka-init-push-to-hamravesh
#
# External service images (tag/push only; no Dockerfile here):
#   make -C identity push-to-hamravesh
#   make -C customer push-to-hamravesh
#   make -C financial-core push-to-hamravesh
#   make -C notification push-to-hamravesh
#
# Override names/versions via env on any sub-make, e.g.:
#   make -C bnpl/hamravesh image TAG=v0.2.0-development BUILD_ENV=development
#   make -C infrastructure/postgres push-to-hamravesh TAG=v0.1.1

.DEFAULT_GOAL := help

.PHONY: help \
	bnpl-image bnpl-push-to-hamravesh bnpl-image-dbadmin bnpl-push-dbadmin-to-hamravesh \
	gateway-image gateway-push-to-hamravesh \
	postgres-image postgres-push-to-hamravesh \
	kafka-init-image kafka-init-push-to-hamravesh \
	loki-image loki-push-to-hamravesh \
	alloy-image alloy-push-to-hamravesh \
	promtail-image promtail-push-to-hamravesh \
	grafana-image grafana-push-to-hamravesh \
	bnpl-image-alloy bnpl-push-alloy-to-hamravesh \
	identity-push-to-hamravesh customer-push-to-hamravesh \
	financial-core-push-to-hamravesh notification-push-to-hamravesh

help:
	@printf '%s\n' \
		'Per-app Makefiles live under each deploy/<app>/ directory.' \
		'Use separate image vs push-to-hamravesh commands (push never rebuilds).' \
		'' \
		'BNPL:' \
		'  make bnpl-image' \
		'  make bnpl-push-to-hamravesh' \
		'  make bnpl-image-alloy' \
		'  make bnpl-push-alloy-to-hamravesh' \
		'  make bnpl-image-dbadmin' \
		'  make bnpl-push-dbadmin-to-hamravesh' \
		'' \
		'Gateway / infra:' \
		'  make gateway-image' \
		'  make gateway-push-to-hamravesh' \
		'  make postgres-image' \
		'  make postgres-push-to-hamravesh' \
		'  make kafka-init-image' \
		'  make kafka-init-push-to-hamravesh' \
		'  make loki-image' \
		'  make loki-push-to-hamravesh' \
		'  make alloy-image' \
		'  make alloy-push-to-hamravesh' \
		'  make promtail-image' \
		'  make promtail-push-to-hamravesh' \
		'  make grafana-image' \
		'  make grafana-push-to-hamravesh' \
		'' \
		'External (push only; tag first if needed):' \
		'  make identity-push-to-hamravesh' \
		'  make customer-push-to-hamravesh' \
		'  make financial-core-push-to-hamravesh' \
		'  make notification-push-to-hamravesh'

bnpl-image:
	$(MAKE) -C bnpl/hamravesh image

bnpl-push-to-hamravesh:
	$(MAKE) -C bnpl/hamravesh push-to-hamravesh

bnpl-image-alloy:
	$(MAKE) -C bnpl/hamravesh image-alloy

bnpl-push-alloy-to-hamravesh:
	$(MAKE) -C bnpl/hamravesh push-alloy-to-hamravesh

bnpl-image-dbadmin:
	$(MAKE) -C bnpl/hamravesh image-dbadmin

bnpl-push-dbadmin-to-hamravesh:
	$(MAKE) -C bnpl/hamravesh push-dbadmin-to-hamravesh

gateway-image:
	$(MAKE) -C gateway image

gateway-push-to-hamravesh:
	$(MAKE) -C gateway push-to-hamravesh

postgres-image:
	$(MAKE) -C infrastructure postgres-image

postgres-push-to-hamravesh:
	$(MAKE) -C infrastructure postgres-push-to-hamravesh

kafka-init-image:
	$(MAKE) -C infrastructure kafka-init-image

kafka-init-push-to-hamravesh:
	$(MAKE) -C infrastructure kafka-init-push-to-hamravesh

loki-image:
	$(MAKE) -C infrastructure loki-image

loki-push-to-hamravesh:
	$(MAKE) -C infrastructure loki-push-to-hamravesh

alloy-image:
	$(MAKE) -C infrastructure alloy-image

alloy-push-to-hamravesh:
	$(MAKE) -C infrastructure alloy-push-to-hamravesh

promtail-image:
	$(MAKE) -C infrastructure promtail-image

promtail-push-to-hamravesh:
	$(MAKE) -C infrastructure promtail-push-to-hamravesh

grafana-image:
	$(MAKE) -C infrastructure grafana-image

grafana-push-to-hamravesh:
	$(MAKE) -C infrastructure grafana-push-to-hamravesh

identity-push-to-hamravesh:
	$(MAKE) -C identity push-to-hamravesh

customer-push-to-hamravesh:
	$(MAKE) -C customer push-to-hamravesh

financial-core-push-to-hamravesh:
	$(MAKE) -C financial-core push-to-hamravesh

notification-push-to-hamravesh:
	$(MAKE) -C notification push-to-hamravesh
