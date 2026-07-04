# ADR 0013: Messaging Abstraction With Kafka and Noop Providers

## Status

Accepted

## Context

The BNPL platform must publish integration events and eventually consume payment-result messages from financial-core systems. The core layer should not import Sarama or Kafka-specific types.

## Decision

Define `pkg/core/messaging` ports:

- `Publisher` — `Publish(ctx, topic, Message) error`
- `Subscriber` — `Subscribe(ctx, Handler) error`
- `Message` — opaque `Key` and `Value` byte slices with optional headers

Implement adapters:

| Provider | Package | Technology |
| --- | --- | --- |
| `noop` | `pkg/adapter/infrastructure/messaging/noop` | No broker; publish no-op, subscribe blocks until context canceled |
| `kafka` | `pkg/adapter/infrastructure/messaging/sarama` | IBM Sarama sync producer and consumer group |

Provider selection: `infrastructure.messaging.provider` (`MESSAGING_PROVIDER`).

Kafka settings under `infrastructure.messaging.kafka` map to `.env.example` `KAFKA_*` variables. Logical topic names (for example `payment_results`) are centralized in config; outbox publisher uses the per-row `destination` string as the publish topic.

Bootstrap constructs both publisher and subscriber at startup. The subscriber **is started** in `bootstrap.startBackgroundWorkers` and routes Financial Core payment-result messages to `financialcorepaymentresultuc.Process`.

### Amendment (2026-06-30)

- Kafka messaging adapters expose `messaging.Health` for broker connectivity checks used by readiness probes (see ADR [0014](0014-health-probes-and-graceful-shutdown.md)).
- Subscriber goroutine starts with bootstrap; consumer still has no domain business handlers.

## Consequences

**Positive**

- Outbox publisher depends on `messaging.Publisher`, not Sarama
- noop allows local runs without Kafka
- Producer acks and retries configurable for financial events

**Negative**

- Single broker entry in YAML (`KAFKA_BROKER_1`) — multi-broker lists need config extension
- Consumer wiring incomplete
- No schema registry or Avro support

**Operational impact**

- Production financial events should use `KAFKA_PRODUCER_REQUIRED_ACKS=-1`
- Kafka must be reachable when provider is `kafka` and outbox rows exist

## Alternatives Considered

**NATS or RabbitMQ**

- Not selected; Kafka already present in local Compose and organizational standards

**Publish directly from HTTP handlers**

- Rejected for integration events; outbox ADR covers reliability

## Related ADRs

- [0009](0009-provider-based-infrastructure-configuration.md)
- [0012](0012-transactional-outbox-publisher.md)
