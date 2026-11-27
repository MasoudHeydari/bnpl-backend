# dpay Central Grafana Alloy

Own image: `registry.hamdocker.ir/parstis-technology/dpay-alloy:v0.1.0`

Central Alloy receives Loki push from Edge collectors (BNPL Edge Alloy today;
other services later), normalizes JSON when possible, applies low-cardinality
labels + structured metadata, and forwards to Loki.

## Why Central Alloy (not Promtail file scrape)

Hamravesh Darkoob disks are **per-app**. Promtail could not read BNPL’s log
files on another app’s disk. Edges push over HTTP to this service instead.

Local Promtail under `assets/local-development/` is unchanged.

## Architecture

```text
Edge Alloy(s) --Loki push--> Central Alloy :3500
                               │  loki.process (normalize)
                               ▼
                             Loki :3100
                               ▲
                            Grafana
```

## Build / push

```bash
cd deploy/infrastructure/alloy
make image
make push-to-hamravesh
# or: make -C deploy alloy-image && make -C deploy alloy-push-to-hamravesh
```

Pin: `docker.arvancloud.ir/grafana/alloy:v1.22.0` (same as Edge).

Config is baked at `/etc/alloy/config.alloy` (no host bind mounts).

## Environment

| Key | Default / example |
| --- | --- |
| `LOKI_PUSH_URL` | `http://loki.parstis-technology-dpay-dev.svc:3100/loki/api/v1/push` |

## Ports (internal only)

| Port | Use |
| --- | --- |
| `3500` | Loki push ingest (`loki.source.api`) |
| `12345` | Alloy ready / management HTTP |

Do not publish these to the public internet.

## Persistent disk

Compose volume `dpay_alloy_data` → `/var/lib/alloy/data` (positions / storage).

- Darkoob replica count **1** while the disk is attached.
- Official image user: `alloy` (UID 473). If a fresh disk is root-owned and
  Alloy cannot write, fix ownership once in the panel/terminal.

## Labels and structured metadata

Index labels (low cardinality):

- `service` (from edge, e.g. `bnpl-service`)
- `application_env`
- `level` (normalized upper-case when present)
- `namespace` = `parstis-technology-dpay-dev` (static)

Structured metadata (not stream labels):

- `trace_id`, `request_id`, `correlation_id`, `span_id`

Do **not** promote borrower/customer/contract/payment IDs to stream labels.

### LogQL examples

```logql
{service="bnpl-service"}
```

```logql
{service="bnpl-service", level="ERROR"}
```

```logql
{application_env="development"} | correlation_id="CORRELATION_ID"
```

## Retry / buffering

`loki.write` uses bounded retries (`max_backoff_retries=10`, backoff up to 5m).
WAL for `loki.write` is experimental in Alloy and is **not** enabled here
(would require `--stability.level=experimental`). Expect at-least-once with
possible loss under prolonged Loki outage or disk pressure — no exactly-once
guarantee.

## Deploy order (Darkoob)

1. Confirm Loki ready: `curl -i http://loki.parstis-technology-dpay-dev.svc:3100/ready`
2. `make alloy-image` / `make alloy-push-to-hamravesh`
3. Create/update Darkoob app `alloy` from `deploy/infrastructure/compose.yaml`
4. Attach disk at `/var/lib/alloy/data`, replica=1, set `LOKI_PUSH_URL`
5. `curl -i http://alloy.parstis-technology-dpay-dev.svc:12345/-/ready`
6. Build/push BNPL Alloy image; update BNPL app image + env (`ALLOY_PUSH_URL`, `LOG_*`)
7. Verify Edge ready + Grafana Explore shows BNPL lines
8. Stop/disable the old Promtail app after confirming no duplicate ingestion
9. Do **not** delete Loki or Grafana disks

## Smoke tests

```bash
curl -i http://loki.parstis-technology-dpay-dev.svc:3100/ready
curl -i http://alloy.parstis-technology-dpay-dev.svc:12345/-/ready

# Optional push probe (from any pod that can reach alloy):
curl -s -X POST \
  "http://alloy.parstis-technology-dpay-dev.svc:3500/loki/api/v1/push" \
  -H "Content-Type: application/json" \
  --data-raw '{"streams":[{"stream":{"service":"probe","application_env":"development"},"values":[["'$(date +%s)000000000'","{\"time\":\"'$(date -Iseconds)'\",\"level\":\"info\",\"msg\":\"alloy-probe\"}"]]}]}'
```

Ready ≠ end-to-end delivery; always confirm lines in Grafana.

## Validate config (same Alloy version)

```bash
docker run --rm --entrypoint /bin/alloy \
  docker.arvancloud.ir/grafana/alloy:v1.22.0 \
  fmt /etc/alloy/config.alloy
# or mount local file:
docker run --rm -v "$PWD/config.alloy:/tmp/config.alloy:ro" \
  --entrypoint /bin/alloy docker.arvancloud.ir/grafana/alloy:v1.22.0 \
  fmt /tmp/config.alloy
```

## Security

- Keep ports internal to the namespace.
- Private networking ≠ authentication; add Loki multi-tenant auth before
  exposing ingest beyond trusted edges.
- Retention stays with existing Loki config (744h) unless intentionally changed.

## Rollback

- Stop receiving by reverting BNPL to the non-Alloy image.
- Leave Central Alloy + Loki data intact; disable the Alloy app only after
  edges stop pushing.
