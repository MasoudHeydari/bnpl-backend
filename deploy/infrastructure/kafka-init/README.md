# Kafka bootstrap one-shot image

The script is baked into this image, avoiding unsupported host bind mounts.
Target broker default: `kafka.parstis-technology-dpay-dev.svc:29092`
(override with `KAFKA_BOOTSTRAP_SERVER`).

Topic list comes from the `kafka-init` job ENV `KAFKA_TOPICS` in
`../jobs.yaml` (v1 names). Keep that list aligned with `.env.example`.

For renaming topics on an already-running broker, use the one-shot script
`scripts/kafka/rename-topics-v1.sh` (not this image).

```bash
cd deploy/infrastructure/kafka-init
make image
make push-to-hamravesh
```
