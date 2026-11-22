# Kafka helper scripts

| Script | Role |
| --- | --- |
| `init-topics.sh` | Greenfield create/ensure (used by local compose `kafka-init`) |
| `rename-topics-v1.sh` | One-shot migration: create v1 names, delete old topics |

Hamravesh bake path: `deploy/infrastructure/kafka-init/` copies its own
`init-topics.sh` into the `dpay-kafka-init` image. Keep topic lists in
`deploy/infrastructure/jobs.yaml` and `.env.example` aligned.

## One-shot rename (existing broker)

Kafka cannot rename topics. `rename-topics-v1.sh` creates each new topic, then
deletes the old name when `DELETE_OLD=1` (default).

Inside the Kafka container (bootstrap defaults to `localhost:29092`):

```bash
# copy script in, then:
DRY_RUN=1 bash /tmp/rename-topics-v1.sh   # preview
DELETE_OLD=1 bash /tmp/rename-topics-v1.sh

# Hamravesh DNS from another pod:
KAFKA_BOOTSTRAP_SERVER=kafka.parstis-technology-dpay-dev.svc:29092 \
  bash rename-topics-v1.sh
```

After migration, redeploy BNPL with updated `KAFKA_TOPIC_*` / consumer topic
env, rebuild/push `dpay-kafka-init`, and run the kafka-init job so greenfield
creates use the new list only.
