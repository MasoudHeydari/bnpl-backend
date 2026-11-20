#!/usr/bin/env python3
"""Interactive admin merchant credit-kinds smoke against a live BNPL API.

Flow:
  1. Choose merchant from local-dev fixtures (Damz / Digikala)
  2. GET  /api/v1/admin/merchants/{merchant_profile_id}/credit-kinds
  3. Choose credit kind (1 = pay_in_installments, 2 = pay_next_month)
  4. Choose desired state (1 = enable, 2 = disable)
  5. PUT  /api/v1/admin/merchants/{merchant_profile_id}/credit-kinds/{credit_kind}
  6. GET again to show the resulting eligibility

Admin routes trust gateway X-Auth-* headers. Hit BNPL directly
(default http://127.0.0.1:7080), not Envoy — Envoy strips inbound X-Auth
and expects a real JWT.

Merchant profile IDs come from pkg/adapter/db/postgres/migrationsrp/000_dev.sql
(internal merchant_profiles.id, not external_merchant_id).

Config (env, optional repo .env):
  BNPL_API_BASE                 default http://127.0.0.1:7080
  BNPL_ADMIN_SUBJECT_ID         optional admin subject UUID (random if unset)
  BNPL_ADMIN_READ_PERM          default bnpl:merchants:read:any
  BNPL_ADMIN_UPDATE_PERM        default bnpl:merchants:credit-kinds:update:any
  BNPL_CA_CERT / BNPL_SSL_INSECURE  same TLS helpers as other smoke scripts
"""

from __future__ import annotations

import os
import re
import ssl
import sys
import time
import uuid
from dataclasses import dataclass
from pathlib import Path
from typing import Any

SCRIPTS_DIR = Path(__file__).resolve().parents[1]
if str(SCRIPTS_DIR) not in sys.path:
    sys.path.insert(0, str(SCRIPTS_DIR))

from lib import cli_style  # noqa: E402
from lib.api_client import APIClient  # noqa: E402
from lib.interactive import select  # noqa: E402

REPO_ROOT = Path(__file__).resolve().parents[3]
DEV_FIXTURE_SQL = (
    REPO_ROOT / "pkg/adapter/db/postgres/migrationsrp/000_dev.sql"
)
REQUEST_DELAY_SECONDS = 0.5

# Fallback when 000_dev.sql cannot be parsed (must match that seed).
FALLBACK_MERCHANTS: list[tuple[str, str, str]] = [
    (
        "Damz",
        "dc520782-b4a1-4c3e-840a-8ce04f7bfacb",
        "1e7c3dd4-0e8b-4ec1-aede-2a119f3ffecc",
    ),
    (
        "Digikala",
        "2813b97e-086f-47d2-b9c0-48d74e32a9c7",
        "27afba8d-9fa8-400d-853a-fdf35619ad3a",
    ),
]

@dataclass(frozen=True)
class DevMerchant:
    display_name: str
    merchant_profile_id: str
    external_merchant_id: str


def load_dotenv(path: Path) -> None:
    if not path.is_file():
        return

    for raw in path.read_text(encoding="utf-8").splitlines():
        line = raw.strip()
        if not line or line.startswith("#") or "=" not in line:
            continue

        key, value = line.split("=", 1)
        key = key.strip()
        value = value.strip().strip("'").strip('"')
        os.environ.setdefault(key, value)


def optional_env(name: str, default: str = "") -> str:
    return os.environ.get(name, default).strip()


def candidate_ca_files() -> list[Path]:
    candidates: list[Path] = []

    explicit = optional_env("BNPL_CA_CERT")
    if explicit:
        candidates.append(Path(explicit).expanduser())

    mkcert_root = Path.home() / ".local/share/mkcert/rootCA.pem"
    candidates.append(mkcert_root)
    candidates.append(REPO_ROOT / "assets/local-development/certs/ca.crt")
    return candidates


def build_ssl_context() -> ssl.SSLContext | None:
    if optional_env("BNPL_SSL_INSECURE") in {"1", "true", "TRUE", "yes", "YES"}:
        ctx = ssl.create_default_context()
        ctx.check_hostname = False
        ctx.verify_mode = ssl.CERT_NONE
        return ctx

    for ca in candidate_ca_files():
        if ca.is_file():
            return ssl.create_default_context(cafile=str(ca))

    return None


def pause(seconds: float = REQUEST_DELAY_SECONDS) -> None:
    print(cli_style.muted(f"\nwaiting {seconds:g}s before next request..."))
    time.sleep(seconds)


def data_of(response: dict[str, Any]) -> dict[str, Any]:
    data = response.get("data")
    if not isinstance(data, dict):
        raise SystemExit("response missing data object")

    return data


def load_dev_merchants(path: Path = DEV_FIXTURE_SQL) -> list[DevMerchant]:
    """Parse merchant_profiles rows from the local-dev seed SQL."""
    if not path.is_file():
        return [
            DevMerchant(name, profile_id, external_id)
            for name, profile_id, external_id in FALLBACK_MERCHANTS
        ]

    text = path.read_text(encoding="utf-8")
    marker = "INSERT INTO bnpl.merchant_profiles"
    start = text.find(marker)
    if start < 0:
        return [
            DevMerchant(name, profile_id, external_id)
            for name, profile_id, external_id in FALLBACK_MERCHANTS
        ]

    # Values block until the next blank-line-separated section / next INSERT.
    chunk = text[start:]
    next_insert = chunk.find("\nINSERT INTO", len(marker))
    if next_insert > 0:
        chunk = chunk[:next_insert]

    row_re = re.compile(
        r"\(\s*'([0-9a-fA-F-]{36})'\s*,\s*'([0-9a-fA-F-]{36})'\s*,\s*'([^']+)'"
    )
    merchants: list[DevMerchant] = []
    for match in row_re.finditer(chunk):
        merchants.append(
            DevMerchant(
                display_name=match.group(3),
                merchant_profile_id=match.group(1),
                external_merchant_id=match.group(2),
            )
        )

    if not merchants:
        return [
            DevMerchant(name, profile_id, external_id)
            for name, profile_id, external_id in FALLBACK_MERCHANTS
        ]

    return merchants


def prompt_merchant(merchants: list[DevMerchant]) -> DevMerchant:
    return select(
        "Select merchant (dev fixtures):",
        [
            (
                f"{merchant.display_name}  "
                f"merchant_profile_id={merchant.merchant_profile_id}",
                merchant,
            )
            for merchant in merchants
        ],
    )


def prompt_credit_kind() -> str:
    return select(
        "Select credit kind:",
        [
            ("pay_in_installments", "pay_in_installments"),
            ("pay_next_month", "pay_next_month"),
        ],
    )


def prompt_enabled() -> bool:
    return select(
        "Select desired eligibility:",
        [
            ("enable", True),
            ("disable", False),
        ],
    )


def admin_headers(subject_id: str, permissions: str) -> dict[str, str]:
    return {
        "x-auth-subject-id": subject_id,
        "x-auth-tenant-id": str(uuid.uuid4()),
        "x-auth-context-id": str(uuid.uuid4()),
        "x-auth-permissions": permissions,
    }


def print_credit_kinds(data: dict[str, Any]) -> None:
    kinds = data.get("credit_kinds")
    if not isinstance(kinds, list):
        print(cli_style.warn("response has no credit_kinds list"))
        return

    print(cli_style.muted("\ncurrent credit_kinds:"))
    for item in kinds:
        if not isinstance(item, dict):
            continue

        print(
            cli_style.muted(
                f"  - {item.get('credit_kind')}: enabled={item.get('enabled')}"
            )
        )


def main() -> None:
    load_dotenv(REPO_ROOT / ".env")

    base_url = optional_env("BNPL_API_BASE", "http://127.0.0.1:7080")
    subject_id = optional_env("BNPL_ADMIN_SUBJECT_ID") or str(uuid.uuid4())
    read_perm = optional_env("BNPL_ADMIN_READ_PERM", "bnpl:merchants:read:any")
    update_perm = optional_env(
        "BNPL_ADMIN_UPDATE_PERM",
        "bnpl:merchants:credit-kinds:update:any",
    )
    permissions = ",".join(
        perm for perm in (read_perm, update_perm) if perm
    )

    merchants = load_dev_merchants()
    print(
        cli_style.muted(
            f"loaded {len(merchants)} merchant(s) from "
            f"{DEV_FIXTURE_SQL.relative_to(REPO_ROOT)}"
        )
    )

    merchant = prompt_merchant(merchants)
    print(
        cli_style.ok(
            f"\nusing {merchant.display_name} "
            f"merchant_profile_id={merchant.merchant_profile_id}"
        )
    )

    client = APIClient(base_url, build_ssl_context())
    headers = admin_headers(subject_id, permissions)
    base_path = (
        f"/api/v1/admin/merchants/{merchant.merchant_profile_id}/credit-kinds"
    )

    pause()
    print(cli_style.step("1) GET credit-kinds"))
    _, get_resp = client.request("GET", base_path, headers=headers)
    get_data = data_of(get_resp)
    print_credit_kinds(get_data)

    credit_kind = prompt_credit_kind()
    enabled = prompt_enabled()
    action = "enable" if enabled else "disable"

    pause()
    print(cli_style.step(f"2) PUT credit-kinds/{credit_kind} ({action})"))
    _, put_resp = client.request(
        "PUT",
        f"{base_path}/{credit_kind}",
        body={"enabled": enabled},
        headers=headers,
    )
    put_data = data_of(put_resp)
    print(
        cli_style.ok(
            f"\nput result credit_kind={put_data.get('credit_kind')!r} "
            f"enabled={put_data.get('enabled')!r} "
            f"changed={put_data.get('changed')!r}"
        )
    )

    pause()
    print(cli_style.step("3) GET credit-kinds (verify)"))
    _, verify_resp = client.request("GET", base_path, headers=headers)
    verify_data = data_of(verify_resp)
    print_credit_kinds(verify_data)

    print(cli_style.ok("\nDone."))
    cli_style.print_json(
        {
            "merchant": merchant.display_name,
            "merchant_profile_id": merchant.merchant_profile_id,
            "credit_kind": credit_kind,
            "enabled": enabled,
            "put": put_data,
            "credit_kinds": verify_data.get("credit_kinds"),
        }
    )


if __name__ == "__main__":
    try:
        main()
    except KeyboardInterrupt:
        print(cli_style.err("\naborted"), file=sys.stderr)
        raise SystemExit(130) from None
