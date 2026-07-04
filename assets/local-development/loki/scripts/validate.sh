#!/usr/bin/env bash
# Validate pack YAML / Compose interpolation / Promtail syntax.
# Does not start the full stack. Does not touch Kafka/DB/app secrets.
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

python3 - <<'PY'
import sys, yaml
from pathlib import Path
root = Path('.')
for rel in [
    'docker-compose.yml',
    'loki/local-config.yaml',
    'promtail/config.yml',
    'grafana/provisioning/datasources/loki.yml',
]:
    path = root / rel
    yaml.safe_load(path.read_text())
    print(f'OK yaml {rel}')
PY

python3 scripts/check-selection.py

if [[ ! -f .env ]]; then
  echo "NOTE: .env missing — copy .env.example and set service log paths before compose config"
else
  if command -v docker >/dev/null 2>&1; then
    if [[ -f secrets/bnpl_loki_grafana_password ]]; then
      docker compose --env-file .env config --quiet
      echo "OK docker compose config"
    else
      echo "NOTE: secrets/bnpl_loki_grafana_password missing — skip compose config"
    fi
  else
    echo "NOTE: docker not in PATH — skip compose config"
  fi
fi

# Promtail -config.expand-env=true -check-syntax if a binary is available (local download or image).
if [[ -x "${PROMTAIL_BIN:-}" ]]; then
  "$PROMTAIL_BIN" -config.file="$ROOT/promtail/config.yml" -config.expand-env=true -check-syntax
  echo "OK promtail -config.expand-env=true -check-syntax via PROMTAIL_BIN"
elif command -v docker >/dev/null 2>&1; then
  # Optional: dry syntax check inside the pinned image (needs pull/mirror).
  if docker image inspect docker.arvancloud.ir/grafana/promtail:3.6.8 >/dev/null 2>&1 \
     || docker image inspect grafana/promtail:3.6.8 >/dev/null 2>&1; then
    IMG=docker.arvancloud.ir/grafana/promtail:3.6.8
    docker image inspect "$IMG" >/dev/null 2>&1 || IMG=grafana/promtail:3.6.8
    docker run --rm --entrypoint /usr/bin/promtail \
      -v "$ROOT/promtail/config.yml:/etc/promtail/config.yml:ro" \
      "$IMG" -config.file=/etc/promtail/config.yml -config.expand-env=true -check-syntax
    echo "OK promtail -config.expand-env=true -check-syntax via docker image $IMG"
  else
    echo "NOTE: promtail image not present locally — skip -config.expand-env=true -check-syntax"
  fi
else
  echo "NOTE: no promtail binary/image — skip -config.expand-env=true -check-syntax"
fi

echo "validate.sh finished"
