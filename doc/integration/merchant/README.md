# Merchant participation and workspace access

The authoritative participation contract is [Merchant gRPC v1](bnpl-participation-grpc.fa.md).
The ordinary BNPL runtime consumes Merchant requests alongside its existing
consumers and jobs. Merchant is an external provider (`grpc` or `noop`); there
is no provisioning-only runtime and no Sandbox-specific domain policy.

## Development deployment

Use an isolated disposable database, separate credentials and topics, and the
appropriate development Financial Core and other service dependencies. The
standard `bnpl-dbadmin db init-dev` command initializes the schema and fixtures;
**it destroys the configured development schema**. The baseline schema changed
for this pre-release integration; never run it on a shared or valuable database.

Generate code with the existing tools before building or testing:

```sh
make proto
make mock
make test-all
```

Configure the existing runtime with its normal settings plus:

```dotenv
APP_ENVIRONMENT=development
EXTERNAL_MERCHANT_PROVIDER=grpc
EXTERNAL_MERCHANT_GRPC_ADDRESS=<merchant-private-host:port>
EXTERNAL_MERCHANT_GRPC_TIMEOUT=5s
EXTERNAL_MERCHANT_GRPC_CONNECTION_TIMEOUT=5s
MERCHANT_PARTICIPATION_ENVIRONMENT=SANDBOX
EXTERNAL_MERCHANT_GRPC_TLS_CA_FILE=<ca-file>
EXTERNAL_MERCHANT_GRPC_TLS_CERT_FILE=<bnpl-client-cert-file>
EXTERNAL_MERCHANT_GRPC_TLS_KEY_FILE=<bnpl-client-key-file>
EXTERNAL_MERCHANT_GRPC_TLS_SERVER_NAME=<merchant-certificate-dns-name>
CIRCUIT_BREAKER_PROVIDER=gobreaker
KAFKA_TOPIC_MERCHANT_PARTICIPATION_REQUESTS=merchant.business.line.participation.requests.v1
KAFKA_CONSUMER_TOPIC_MERCHANT_PARTICIPATION_REQUESTS=merchant.business.line.participation.requests.v1
TASK_MERCHANT_PROGRESS_DELIVERY_ENABLED=true
TASK_MERCHANT_PROGRESS_DELIVERY_HANDLER=merchant_progress_delivery
```

Keep the other configured consumer subscriptions and scheduled tasks. The
optional participation subscription is appended at consumer construction without
mutating configuration. Set an appropriate isolated Kafka group and quarantine
configuration. The default example uses the noop provider, no participation
subscription and disabled delivery. SANDBOX requires development build/runtime;
LIVE is rejected by configuration until separately agreed and tested.

## Participation and declarations

The gRPC adapter uses existing mTLS, RPC deadlines and circuit breaker. The
consumer validates the configured environment and matches the authoritative
request, application, merchant and declaration revision. An older event does
not evaluate a newer declaration; a future event waits for the reference.

The platform policy permits positive IRR amounts up to 5,000,000,000 minor units
and terms up to 12 months. Accepted declared terms are stored unchanged. Missing
or invalid declarations produce a durable CHANGES_REQUESTED outcome instead of
creating an account or quarantining a business rejection. Malformed envelopes
and contradictory identities remain quarantine candidates.

Request identity, immutable evaluation (request and declaration revision), and
immutable progress delivery (event ID and owner version) are persisted together
with the processed inbox record. Retries reuse the original report. A corrected
declaration under the same request can be evaluated again. No terminal, outlet,
financial entitlement or purchase source is created by participation READY.
The existing funding, settlement, risk, jurisdiction and installment credit
policy defaults remain unchanged. The outlet eligibility RPC is a reference
operation only, not a user authorization check or an automatic setup operation.

## Panel authorization

The Gateway must resolve Merchant workspace on every protected panel request
and inject `x-auth-merchant-id`, `x-auth-outlet-scope`, and `x-auth-outlet-ids`
after stripping client values. IDs are canonical UUIDs; the outlet list is a
JSON array, not CSV. `ALL_OUTLETS` requires an empty list; `SELECTED_OUTLETS`
with an empty list returns no outlet-scoped data. Merchant ID is not Subject ID.
BNPL retains its existing operation permissions. Outlet restrictions are applied
in SQL before counts and pagination; the external branch ID must be the actual
Merchant outlet ID, never inferred from an online store ID. Terminal and API key
operations require merchant-wide scope because the current terminal is owned by
Merchant, not an outlet. Partner API authentication remains separate.

The Gateway authorization adapter is still pending. Both supplied Envoy configs
strip the three client headers; neither synthesizes them or grants access.
Before enabling panel routes, verify trusted Gateway-only access to the BNPL
listener (including direct network access restrictions), authenticated upstream
connection, header stripping and per-request workspace resolution. Verify two
merchants, two outlets, empty selected scope, forbidden terminal operations and
invalid/missing headers in shared Dev. Never open the panel based on forged or
client-provided workspace headers.
