#!/usr/bin/env bash
# Create the topics required by the local BNPL integration environment.
# This is intentionally a one-shot initialization task, not a broker entrypoint.
set -euo pipefail

bootstrap_server="kafka:29092"
partitions=3
replication_factor=1

: "${KAFKA_TOPICS:?KAFKA_TOPICS must contain the local topic names}"

# Compose folds KAFKA_TOPICS into a space-separated list. Do not evaluate it as shell code.
read -r -a topics <<< "$KAFKA_TOPICS"
if (( ${#topics[@]} == 0 )); then
  echo 'No Kafka topics were configured' >&2
  exit 1
fi

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

  printf 'Ensuring local Kafka topic exists: %s\n' "$topic"
  /opt/kafka/bin/kafka-topics.sh \
    --bootstrap-server "$bootstrap_server" \
    --create \
    --if-not-exists \
    --topic "$topic" \
    --partitions "$partitions" \
    --replication-factor "$replication_factor"
done

echo 'Local Kafka topic initialization completed.'
