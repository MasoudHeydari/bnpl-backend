#!/usr/bin/env bash
# One-shot Kafka topic migration for BNPL / Parstis v1 naming.
#
# Kafka has no native rename. This script:
#   1) creates each destination topic (3 partitions, RF=1)
#   2) optionally deletes the old topic when it differs and still exists
#
# Intended to run inside the Kafka broker container:
#   kubectl exec -it <kafka-pod> -- bash /tmp/rename-topics-v1.sh
#   # or docker compose exec kafka bash /path/to/rename-topics-v1.sh
#
# Defaults assume in-container loopback. Override:
#   KAFKA_BOOTSTRAP_SERVER=kafka.parstis-technology-dpay-dev.svc:29092
#   DELETE_OLD=1          # delete old topics after create (default: 1)
#   DRY_RUN=1             # print actions only
set -euo pipefail

bootstrap_server="${KAFKA_BOOTSTRAP_SERVER:-localhost:29092}"
partitions="${KAFKA_PARTITIONS:-3}"
replication_factor="${KAFKA_REPLICATION_FACTOR:-1}"
delete_old="${DELETE_OLD:-1}"
dry_run="${DRY_RUN:-0}"

topics_sh="/opt/kafka/bin/kafka-topics.sh"
if [[ ! -x "$topics_sh" ]]; then
	printf 'kafka-topics.sh not found at %s (run inside the Kafka image)\n' "$topics_sh" >&2
	exit 1
fi

# old -> new. Identical pairs are create-only (unchanged topics).
# notification.sms.requests is retired (no destination); commands.v1 already exists.
declare -a RENAMES=(
	"financial.events=financial.events.v1"
	"financial.provider.commands=financial.provider.commands.v1"
	"document.lifecycle=document.lifecycle.v1"
	"document.owner.lifecycle=document.owner.lifecycle.v1"
	"bnpl.merchant.checkout.events=bnpl.merchant.checkout.events.v1"
	"financial-core.payment-results=financial.core.payment.results.v1"
	"financial_core.payment_application_results=financial.core.payment.application.results.v1"
	"financial_core.installment_payment_requests=financial.core.installment.payment.requests.v1"
	"bnpl.installment_payment_reconciliation_requests=bnpl.installment.payment.reconciliation.requests.v1"
	"financial_core.installment_payment_reconciliation_results=financial.core.installment.payment.reconciliation.results.v1"
	"bnpl.installment_payment_reconciliation_acknowledgements=bnpl.installment.payment.reconciliation.acknowledgements.v1"
)

# Ensure-only (no rename). Creating a topic does not wire producers/consumers.
declare -a ENSURE=(
	"bnpl.audit.events.v1"
	"bnpl.consumer.quarantine.v1"
	"identity.auth.revocations.v1"
	"identity.outbox.v1"
	"notification.commands.v1"
	"parstis.notification.lifecycle.v1"
)

# Retired topics to delete when DELETE_OLD=1 (no v1 replacement).
declare -a RETIRE=(
	"notification.sms.requests"
)

run() {
	if [[ "$dry_run" == "1" ]]; then
		printf 'DRY_RUN: %s\n' "$*"
		return 0
	fi
	"$@"
}

topic_exists() {
	local topic="$1"
	"$topics_sh" --bootstrap-server "$bootstrap_server" --list 2>/dev/null | grep -Fxq -- "$topic"
}

topic_partition_count() {
	local topic="$1"
	"$topics_sh" \
		--bootstrap-server "$bootstrap_server" \
		--describe \
		--topic "$topic" \
		2>/dev/null |
		awk -F'\t' '
        /^Topic: / && $0 !~ /Partition:/ {
          for (i = 1; i <= NF; i++) {
            if ($i ~ /^PartitionCount: /) {
              sub(/^PartitionCount: /, "", $i)
              print $i
              exit
            }
          }
        }
      '
}

ensure_topic() {
	local topic="$1"

	if [[ ! "$topic" =~ ^[A-Za-z0-9._-]+$ ]]; then
		printf 'Invalid Kafka topic name: %s\n' "$topic" >&2
		exit 1
	fi

	printf 'Ensure topic: %s\n' "$topic"
	run "$topics_sh" \
		--bootstrap-server "$bootstrap_server" \
		--create \
		--if-not-exists \
		--topic "$topic" \
		--partitions "$partitions" \
		--replication-factor "$replication_factor"

	if [[ "$dry_run" == "1" ]]; then
		return 0
	fi

	local current
	current="$(topic_partition_count "$topic" || true)"
	if [[ -z "$current" ]]; then
		printf 'Unable to read PartitionCount for topic %s\n' "$topic" >&2
		exit 1
	fi

	if ((current < partitions)); then
		printf 'Increase partitions %s: %s -> %s\n' "$topic" "$current" "$partitions"
		run "$topics_sh" \
			--bootstrap-server "$bootstrap_server" \
			--alter \
			--topic "$topic" \
			--partitions "$partitions"
	fi
}

delete_topic_if_exists() {
	local topic="$1"

	if [[ "$dry_run" == "1" ]]; then
		printf 'DRY_RUN: would delete topic if present: %s\n' "$topic"
		return 0
	fi

	if ! topic_exists "$topic"; then
		printf 'Skip delete (missing): %s\n' "$topic"
		return 0
	fi

	printf 'Delete topic: %s\n' "$topic"
	run "$topics_sh" \
		--bootstrap-server "$bootstrap_server" \
		--delete \
		--topic "$topic"
}

printf 'Kafka bootstrap: %s (DELETE_OLD=%s DRY_RUN=%s)\n' \
	"$bootstrap_server" "$delete_old" "$dry_run"

declare -A seen=()

for pair in "${RENAMES[@]}"; do
	old="${pair%%=*}"
	new="${pair#*=}"

	ensure_topic "$new"
	seen["$new"]=1

	if [[ "$old" != "$new" && "$delete_old" == "1" ]]; then
		delete_topic_if_exists "$old"
	fi
done

for topic in "${ENSURE[@]}"; do
	if [[ -n "${seen[$topic]:-}" ]]; then
		continue
	fi
	ensure_topic "$topic"
	seen["$topic"]=1
done

if [[ "$delete_old" == "1" ]]; then
	for topic in "${RETIRE[@]}"; do
		delete_topic_if_exists "$topic"
	done
fi

echo 'Kafka topic v1 migration completed.'
echo 'Next: deploy BNPL/FC/Notification with updated KAFKA_TOPIC_* env, then rebuild kafka-init image for greenfield creates.'
