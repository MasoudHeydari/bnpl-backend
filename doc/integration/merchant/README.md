# Merchant participation provisioning — isolated Dev pilot

Contract: [final Merchant agreement](merchant-bnpl-dev-pilot-final-fa.md).

The opt-in provisioning-only runtime consumes one SANDBOX topic, resolves the
request through Merchant, creates the BNPL profile and installment eligibility,
and records the immutable READY report in the same transaction as the inbox.
It runs the delivery worker and health endpoints, not panel, checkout, financial
clients or other business jobs. There is one account per Merchant in this
isolated database; no terminal, credential, or purchase source is created.
The existing `display_name` column is a local projection of `trade_name`;
legal name and other public fields are not inferred. Existing non-pilot routes
are unchanged. Do not use this runtime for LIVE or enable normal business routes
until the remaining authorization and financial contracts are implemented.

## Deployment prerequisites

Use a dedicated disposable database and isolated PostgreSQL managed roles (a
separate cluster is recommended), an empty dedicated participation topic and a
separate credential namespace. Use the existing development initializer with
its database-admin config pointed only to the new isolated database:

```sh
bnpl-dbadmin --config /path/to/sandbox-dbadmin.yaml db init-dev
```

`init-dev` destructively recreates the managed schema and loads the standard
Dev fixtures. This is intentionally the same database initializer as ordinary
Dev. The pilot merchant must have a new ID distinct from seeded fixtures. Never
run this command against shared Dev or any non-disposable database.

In the standard BNPL runtime configuration, set the following (alongside the
normal isolated database, logging, health and infrastructure settings):

```dotenv
APP_ENVIRONMENT=development
MERCHANT_SANDBOX_ENABLED=true
MERCHANT_SANDBOX_BASE_URL=https://<approved-merchant-internal-origin>
MERCHANT_SANDBOX_TOKEN=<service-token-from-deployment-secret>
EXTERNAL_FINANCIAL_CORE_PROVIDER=noop
HTTP_CLIENT_PROVIDER=standard
CIRCUIT_BREAKER_PROVIDER=gobreaker
MESSAGING_PROVIDER=kafka
TASK_PROVIDER=inprocess
KAFKA_CLIENT_ID=bnpl-sandbox
KAFKA_GROUP_ID=bnpl-sandbox-consumer-group
KAFKA_CONSUMER_INITIAL_OFFSET=oldest
KAFKA_CONSUMER_QUARANTINE_TOPIC=bnpl.sandbox.consumer.quarantine.v1
```

Sandbox activation requires both the development build tag and
`APP_ENVIRONMENT=development`; any other combination fails configuration
validation. Sandbox isolation is a deployment concern, not a financing policy.
Initial merchant terms are owned by BNPL and persisted per merchant profile:
IRR/IR, maximum transaction amount 1,000,000,000 IRR (the lower existing Dev
merchant fixture cap), four-month maximum tenure, platform funding, direct
settlement, low risk, installment credit only. These are initial onboarding
terms for the pilot, not an approved production underwriting policy.
The runtime subscribes only to the agreed participation topic. `oldest` is
appropriate only for the agreed fresh isolated topic; do not replay historic
messages. The HTTP adapter reads the bearer token directly from the process environment
at startup, keeps it redacted and uses the shared named Merchant circuit
breaker. Temporary HTTP/transport failures count toward opening the circuit;
conclusive contract/access errors do not. An open circuit is retryable.
The trusted HTTPS origin and disabled redirects are unchanged; the actual
service identity and claims must be confirmed. Restart/redeploy to rotate the
environment token. Do not send credentials in messages,
configuration committed to git, logs or reports. Kafka authentication TLS/SASL
is not supplied by this patch; confirm and implement the infrastructure's
actual mechanism before connecting to any shared or authenticated broker.

## Durable delivery

`merchant_participation_requests` stores the request identity, profile reference,
progress event ID and owner version. The pilot emits READY directly (version 1),
not intermediate or rejection callbacks. Invalid/conflicting events go through
the existing consumer quarantine policy. A transient callback failure retries
the same snapshot with backoff. A lease recovers work after a crash. Permanent
errors or a mismatched successful response set delivery to `blocked` and require
operator investigation. Correct the cause and use an audited owner-approved
requeue on the isolated database if appropriate; do not change the event ID,
version, or account reference. The worker does not recreate accounts on retry.

## Verification

Run `make test-all` (unit and PostgreSQL integration). Compare a real broker
message against the contract fixture only after infrastructure approval.
Financial Sandbox, permanent purchase-source reference ownership, panel
context/membership authorization, and end-to-end checkout are separate work.
