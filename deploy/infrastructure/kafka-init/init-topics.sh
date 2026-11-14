#!/usr/bin/env bash
# Create the topics required by the local BNPL integration environment.
# This is intentionally a one-shot initialization task, not a broker entrypoint.
#
# Important: --create --if-not-exists does NOT change partition count on an
# already-existing topic. Producers (or auto-create) can therefore leave a
# topic at the broker default of 1 partition. After create/ensure, this script
# alters the topic up to the desired partition count when needed.
set -euo pipefail

bootstrap_server="kafka.parstis-technology-dpay-dev.svc:29092"
partitions=3
replication_factor=1

: "${KAFKA_TOPICS:?KAFKA_TOPICS must contain the local topic names}"

# Compose folds KAFKA_TOPICS into a space-separated list. Do not evaluate it as shell code.
read -r -a topics <<<"$KAFKA_TOPICS"
if ((${#topics[@]} == 0)); then
	echo 'No Kafka topics were configured' >&2
	exit 1
fi

topic_partition_count() {
	local topic="$1"
	/opt/kafka/bin/kafka-topics.sh \
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

	printf 'Ensuring local Kafka topic exists: %s\n' "$topic"
	/opt/kafka/bin/kafka-topics.sh \
		--bootstrap-server "$bootstrap_server" \
		--create \
		--if-not-exists \
		--topic "$topic" \
		--partitions "$partitions" \
		--replication-factor "$replication_factor"

	local current
	current="$(topic_partition_count "$topic" || true)"
	if [[ -z "$current" ]]; then
		printf 'Unable to read PartitionCount for topic %s\n' "$topic" >&2
		exit 1
	fi

	if ((current < partitions)); then
		printf 'Increasing partitions for %s: %s -> %s\n' "$topic" "$current" "$partitions"
		/opt/kafka/bin/kafka-topics.sh \
			--bootstrap-server "$bootstrap_server" \
			--alter \
			--topic "$topic" \
			--partitions "$partitions"
	elif ((current > partitions)); then
		printf 'Topic %s already has %s partitions (desired %s); leaving unchanged\n' \
			"$topic" "$current" "$partitions"
	fi
}

declare -A created=()
for topic in "${topics[@]}"; do
	if [[ ! "$topic" =~ ^[A-Za-z0-9._-]+$ ]]; then
		printf 'Invalid Kafka topic name: %s\n' "$topic" >&2
		exit 1
	fi

	# Consumer aliases often point to the same physical topic: visit each only once.
	if [[ -n "${created[$topic]:-}" ]]; then
		continue
	fi
	created["$topic"]=1

	ensure_topic "$topic"
done

echo 'Local Kafka topic initialization completed.'
