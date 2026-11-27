# BNPL Edge Grafana Alloy (Hamravesh)

Edge Alloy runs **inside** the BNPL container under `supervisord`, tails
`LOG_FILE`, and pushes raw lines to Central Alloy. It does not talk to Loki
directly and does not share a disk with other Darkoob apps.

## Why not Promtail on Hamravesh?

Darkoob turns each Compose service into a separate app with **isolated disks**.
Promtail could not see BNPL’s `/var/log/services/...` tree, so `/ready` stayed
`Unable to find any logs to tail`. Edge Alloy + HTTP push avoids shared volumes.

Local development still uses Promtail under `assets/local-development/`; that
migration is out of scope.

## Architecture

```text
BNPL container (supervisord PID 1)
├── Edge Alloy  →  ALLOY_PUSH_URL (Central Alloy :3500)
├── BNPL        →  stdout + LOG_FILE JSONL
└── logrotate   →  copytruncate on bnpl.jsonl
```

Central Alloy → Loki → Grafana (see `deploy/infrastructure/alloy/`).

## Files

| File | Role |
| --- | --- |
| `config.alloy` | Edge `loki.source.file` + `loki.write` |
| `supervisord.conf` | Alloy, BNPL, logrotate, FATAL watchdog |
| `logrotate.conf` | 50 MiB × 5, copytruncate |
| `rotate-logs.sh` | Periodic logrotate loop (bnpluser) |
| `quit-on-bnpl-fatal.sh` | Exit container when BNPL is FATAL |

## Docker build

Stage: `runtime-development-alloy` (does not change default `docker build` target).

```bash
cd deploy/bnpl/hamravesh
make image-alloy
make push-alloy-to-hamravesh
# or from deploy/: make bnpl-image-alloy && make bnpl-push-alloy-to-hamravesh
```

Image: `registry.hamdocker.ir/parstis-technology/bnpl-backend:v0.1.0-development-alloy`

Pin: `grafana/alloy:v1.22.0` (glibc binary + Alpine `gcompat`). Same pin as Central.

One-shot jobs (`jobs.yaml`) keep the non-Alloy runtime/dbadmin images.

## Environment

| Key | Value |
| --- | --- |
| `LOG_PROVIDER` | `slog` |
| `LOG_LEVEL` | `info` (required for single-line JSON) |
| `LOG_FORMAT` | `json` |
| `LOG_FILE` | `/var/log/services/bnpl-service/bnpl.jsonl` |
| `ALLOY_PUSH_URL` | `http://alloy.parstis-technology-dpay-dev.svc:3500/loki/api/v1/push` |
| `APP_ENVIRONMENT` | label source for `application_env` |

### Technical debt: DEBUG pretty JSON

With `LOG_LEVEL=debug`, `newDebugJSONWriter` pretty-prints and colorizes JSON.
That breaks line-oriented collectors. Do **not** enable DEBUG on Hamravesh Alloy
until a separate logger change adds a compact-JSON-at-DEBUG option.

### LOG_FILE failure behavior (Go logger unchanged)

- If `LOG_FILE` cannot be created/opened at startup, logger init fails and BNPL
  does not start.
- Runtime write errors after open are not isolated by a dedicated fallback
  writer; disk-full conditions can surface as log write errors. Alloy/Central
  outages do **not** stop BNPL (file append is local).

## Persistent disk (Darkoob)

Mount `dpay_service_logs` → `/var/log/services` on the **BNPL app only**.

```text
/var/log/services/
├── bnpl-service/bnpl.jsonl (+ rotated archives)
├── alloy-data/             # Edge positions / storage
└── logrotate/status
```

- Replica count must stay **1** while a disk is attached.
- After attaching a new disk, ensure `bnpluser` can write (entrypoint/`MkdirAll`
  create `bnpl-service/`; image seeds ownership, volume mounts can reset it —
  fix ownership once in the Darkoob console/terminal if needed). Do not use 777.
- Edge management UI/ready: `127.0.0.1:12345` only (loopback).

## Supervisor lifecycle

- Priority 10 Alloy starts first, stops last (drain window).
- Priority 20 BNPL starts second, stops first on SIGTERM (`stopwaitsecs=25`).
- Alloy crash restarts Alloy only; BNPL keeps running.
- BNPL unexpected exit retries 3× then FATAL → eventlistener stops supervisord
  so the container exits (health must probe BNPL HTTP, not supervisord).

## Log rotation

- Size 50 MiB, keep 5, check every 60 s (`LOGROTATE_CHECK_INTERVAL_SECONDS`).
- `copytruncate`: possible loss between copy and truncate; if Alloy is down for
  a long time, rotated files may be deleted before ship. No exactly-once claim.
- Rotated `*.gz` / dated archives are **not** scrape targets (Edge tails only
  `bnpl.jsonl`).

## Smoke tests (inside BNPL container)

```bash
ps aux | head
curl -i http://127.0.0.1:12345/-/ready
ls -lah /var/log/services/bnpl-service/
ls -lah /var/log/services/alloy-data/
# Confirm JSONL is single-line:
tail -n 1 /var/log/services/bnpl-service/bnpl.jsonl | jq .
```

Hamravesh panel stdout should still show BNPL logs (MultiWriter → stdout).

## Rollback

Point the BNPL Darkoob app image back to
`bnpl-backend:v0.1.0-development` (non-Alloy). Do not delete Loki/Grafana disks.
Leave Central Alloy running if other edges will use it later, or stop it after
confirming no push traffic.

## Security

- No new secrets in the image.
- Edge Alloy HTTP only on loopback.
- Do not enable HTTP response-body or SQL argument logging.
- Private Darkoob networking is not authentication; production should add Loki
  auth / network policy later.
