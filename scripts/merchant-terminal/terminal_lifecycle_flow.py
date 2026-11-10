#!/usr/bin/env python3
"""Smoke-test merchant terminal panel + partner authenticate flows.

Flow:
  1. GET /api/v1/merchant/terminal
  2. Provision (if empty) OR Rotate grace=0 (if terminal already exists)
  3. Idempotent replay of that issuance (same Idempotency-Key)
  4. Partner auth probe: wrong key → 401
  5. Rotate with short grace → retiring credential present
  6. GET shows retiring; both old and new keys authenticate
  7. Retire retiring credential
  8. Old key → 401; new key still accepted; GET has no retiring
  9. Print final terminal_id + api_key for other partner APIs

Panel routes use synthetic gateway X-Auth-* headers. Hit BNPL directly
(default http://127.0.0.1:8080), not Envoy — Envoy strips inbound X-Auth
and expects a real JWT.

Config (env, optional repo .env):
  BNPL_API_BASE                 default http://127.0.0.1:8080
  BNPL_EXTERNAL_MERCHANT_ID     default Damz seed (1e7c3dd4-…)
  BNPL_MERCHANT_TERMINAL_PERM   default bnpl:merchant-terminals:manage:merchant
  BNPL_ROTATE_GRACE_SECONDS     default 3600 (grace used in step 5)
  BNPL_CA_CERT / BNPL_SSL_INSECURE  same TLS helpers as checkout-flow
"""

from __future__ import annotations

import os
import ssl
import sys
import time
import uuid
from pathlib import Path
from typing import Any

SCRIPTS_DIR = Path(__file__).resolve().parents[1]
if str(SCRIPTS_DIR) not in sys.path:
    sys.path.insert(0, str(SCRIPTS_DIR))

from lib import cli_style  # noqa: E402
from lib.api_client import APIClient  # noqa: E402

REPO_ROOT = Path(__file__).resolve().parents[2]

DEFAULT_EXTERNAL_MERCHANT_ID = "1e7c3dd4-0e8b-4ec1-aede-2a119f3ffecc"
DEFAULT_MANAGE_PERM = "bnpl:merchant-terminals:manage:merchant"
REQUEST_DELAY_SECONDS = 0.5


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


def data_of(response: dict[str, Any]) -> Any:
    if "data" not in response:
        raise SystemExit("response missing data")

    return response["data"]


def panel_headers(external_merchant_id: str, permission: str) -> dict[str, str]:
    return {
        "x-auth-subject-id": external_merchant_id,
        "x-auth-tenant-id": str(uuid.uuid4()),
        "x-auth-context-id": str(uuid.uuid4()),
        "x-auth-permissions": permission,
    }


def partner_headers(terminal_id: str, api_key: str) -> dict[str, str]:
    return {
        "X-Terminal-Id": terminal_id,
        "X-API-Key": api_key,
    }


def probe_partner_auth(
    client: APIClient,
    terminal_id: str,
    api_key: str,
    *,
    expect_unauthorized: bool,
) -> None:
    """Probe PartnerTerminalAuth without creating a real checkout session.

    Hits GET /partner/checkout-sessions/{cs_…} with a well-formed but
    non-existent public token. Middleware runs first:
      - 401 → credentials rejected
      - anything else (typically 404) → credentials accepted; handler ran
    """
    # Canonical public token shape: cs_ + 64 lowercase hex (see CheckoutSessionToken).
    probe_token = "cs_" + ("0" * 64)
    path = f"/api/v1/partner/checkout-sessions/{probe_token}"
    status, _ = client.request(
        "GET",
        path,
        headers=partner_headers(terminal_id, api_key),
        accept_any_status=True,
    )
    unauthorized = status == 401
    if expect_unauthorized and not unauthorized:
        raise SystemExit(
            f"partner auth expected 401 with bad/retired key, got HTTP {status}"
        )

    if not expect_unauthorized and unauthorized:
        raise SystemExit("partner auth unexpected 401 with presented key")

    if unauthorized:
        print(cli_style.ok("partner auth probe: rejected (401)"))
    else:
        print(
            cli_style.ok(
                f"partner auth probe: credentials accepted (HTTP {status}; "
                "session lookup not the point of this step)"
            )
        )


def require_str(data: dict[str, Any], key: str) -> str:
    value = data.get(key)
    if not isinstance(value, str) or not value.strip():
        raise SystemExit(f"response data missing string field {key!r}")

    return value.strip()


def main() -> None:
    load_dotenv(REPO_ROOT / ".env")

    base_url = optional_env("BNPL_API_BASE", "http://127.0.0.1:8080")
    external_merchant_id = optional_env(
        "BNPL_EXTERNAL_MERCHANT_ID",
        DEFAULT_EXTERNAL_MERCHANT_ID,
    )
    manage_perm = optional_env("BNPL_MERCHANT_TERMINAL_PERM", DEFAULT_MANAGE_PERM)
    grace_seconds = int(optional_env("BNPL_ROTATE_GRACE_SECONDS", "3600"))
    if grace_seconds < 1:
        raise SystemExit("BNPL_ROTATE_GRACE_SECONDS must be >= 1 for the grace/retire path")

    stamp = time.strftime("%Y%m%d%H%M%S")
    provision_idem = optional_env(
        "BNPL_PROVISION_IDEMPOTENCY_KEY",
        f"smoke-provision-{stamp}",
    )
    rotate_zero_idem = optional_env(
        "BNPL_ROTATE_ZERO_IDEMPOTENCY_KEY",
        f"smoke-rotate-zero-{stamp}",
    )
    rotate_grace_idem = optional_env(
        "BNPL_ROTATE_GRACE_IDEMPOTENCY_KEY",
        f"smoke-rotate-grace-{stamp}",
    )

    client = APIClient(base_url, build_ssl_context())
    auth = panel_headers(external_merchant_id, manage_perm)

    print(cli_style.step("1) GET /api/v1/merchant/terminal"))
    _, get_resp = client.request(
        "GET",
        "/api/v1/merchant/terminal",
        headers=auth,
        expect_statuses={200},
    )
    terminals = data_of(get_resp)
    if not isinstance(terminals, list):
        raise SystemExit("GET terminal expected data to be a list")

    terminal_id = ""
    api_key = ""
    previous_api_key = ""
    issuance_path = ""
    issuance_idem = ""
    issuance_body: dict[str, Any] | None = None
    final_issue_status = 0

    pause()
    if len(terminals) == 0:
        print(cli_style.step("2) Provision (merchant has no terminal)"))
        issuance_path = "/api/v1/merchant/terminal"
        issuance_idem = provision_idem
        issuance_body = None
        issue_status, provision_resp = client.request(
            "POST",
            issuance_path,
            body={},
            headers={**auth, "Idempotency-Key": issuance_idem},
            expect_statuses={200, 201},
        )
        provision_data = data_of(provision_resp)
        if not isinstance(provision_data, dict):
            raise SystemExit("provision response data must be an object")

        terminal_id = require_str(provision_data, "terminal_id")
        api_key = require_str(provision_data, "api_key")
        final_issue_status = issue_status
        if provision_data.get("replayed") is True:
            print(cli_style.warn("provision returned replayed=true on first call"))
    else:
        first = terminals[0]
        if not isinstance(first, dict):
            raise SystemExit("GET terminal item must be an object")

        terminal_id = require_str(first, "terminal_id")
        print(
            cli_style.step(
                f"2) Terminal already exists ({terminal_id}); "
                "Rotate with grace_period_seconds=0 to obtain a known api_key"
            )
        )
        issuance_path = "/api/v1/merchant/terminal/credentials/rotate"
        issuance_idem = rotate_zero_idem
        issuance_body = {"grace_period_seconds": 0}
        issue_status, rotate_resp = client.request(
            "POST",
            issuance_path,
            body=issuance_body,
            headers={**auth, "Idempotency-Key": issuance_idem},
            expect_statuses={200, 201},
        )
        rotate_data = data_of(rotate_resp)
        if not isinstance(rotate_data, dict):
            raise SystemExit("rotate response data must be an object")

        terminal_id = require_str(rotate_data, "terminal_id")
        api_key = require_str(rotate_data, "api_key")
        final_issue_status = issue_status
        if rotate_data.get("retiring") is not None:
            raise SystemExit("grace=0 rotate must not return retiring")

    print(cli_style.ok(f"issued terminal_id={terminal_id} (HTTP {final_issue_status})"))

    pause()
    print(cli_style.step("3) Idempotent replay of issuance (same Idempotency-Key)"))
    _, replay_resp = client.request(
        "POST",
        issuance_path,
        body=issuance_body if issuance_body is not None else {},
        headers={**auth, "Idempotency-Key": issuance_idem},
        expect_statuses={200, 201},
    )
    replay_data = data_of(replay_resp)
    if not isinstance(replay_data, dict):
        raise SystemExit("replay response data must be an object")

    if replay_data.get("replayed") is not True:
        raise SystemExit("expected replayed=true on idempotent issuance replay")

    if require_str(replay_data, "api_key") != api_key:
        raise SystemExit("replayed api_key does not match original issuance")

    if require_str(replay_data, "terminal_id") != terminal_id:
        raise SystemExit("replayed terminal_id does not match")

    print(cli_style.ok("idempotent replay OK"))

    pause()
    print(cli_style.step("4) Partner authenticate with wrong key → expect 401"))
    probe_partner_auth(
        client,
        terminal_id,
        "bnpl_definitely_wrong_key_for_smoke",
        expect_unauthorized=True,
    )

    pause()
    print(cli_style.step(f"5) Rotate with grace_period_seconds={grace_seconds}"))
    previous_api_key = api_key
    grace_status, grace_resp = client.request(
        "POST",
        "/api/v1/merchant/terminal/credentials/rotate",
        body={"grace_period_seconds": grace_seconds},
        headers={**auth, "Idempotency-Key": rotate_grace_idem},
        expect_statuses={200, 201},
    )
    grace_data = data_of(grace_resp)
    if not isinstance(grace_data, dict):
        raise SystemExit("grace rotate response data must be an object")

    api_key = require_str(grace_data, "api_key")
    final_issue_status = grace_status
    retiring = grace_data.get("retiring")
    if not isinstance(retiring, dict):
        raise SystemExit("grace rotate must return retiring object")

    retiring_credential_id = require_str(retiring, "credential_id")
    print(cli_style.ok(f"active key issued (HTTP {grace_status})"))
    print(cli_style.ok(f"retiring.credential_id={retiring_credential_id}"))

    pause()
    print(cli_style.step("6) GET after grace rotate — expect active + retiring"))
    _, get_after_rotate = client.request(
        "GET",
        "/api/v1/merchant/terminal",
        headers=auth,
        expect_statuses={200},
    )
    after_list = data_of(get_after_rotate)
    if not isinstance(after_list, list) or len(after_list) != 1:
        raise SystemExit("expected exactly one terminal after rotate")

    after = after_list[0]
    if not isinstance(after, dict):
        raise SystemExit("GET item must be an object")

    if after.get("retiring") is None:
        raise SystemExit("GET after grace rotate missing retiring")

    if require_str(after["retiring"], "credential_id") != retiring_credential_id:
        raise SystemExit("GET retiring.credential_id mismatch")

    pause()
    print(cli_style.step("6a) Partner auth with previous (retiring) key → accepted during grace"))
    probe_partner_auth(
        client,
        terminal_id,
        previous_api_key,
        expect_unauthorized=False,
    )

    pause()
    print(cli_style.step("6b) Partner auth with new active key → accepted"))
    probe_partner_auth(client, terminal_id, api_key, expect_unauthorized=False)

    pause()
    print(cli_style.step(f"7) Retire retiring credential {retiring_credential_id}"))
    _, retire_resp = client.request(
        "POST",
        f"/api/v1/merchant/terminal/credentials/{retiring_credential_id}/retire",
        body={},
        headers=auth,
        expect_statuses={200},
    )
    retire_data = data_of(retire_resp)
    if not isinstance(retire_data, dict):
        raise SystemExit("retire response data must be an object")

    if require_str(retire_data, "credential_id") != retiring_credential_id:
        raise SystemExit("retire credential_id mismatch")

    if not retire_data.get("retired_at"):
        raise SystemExit("retire response missing retired_at")

    pause()
    print(cli_style.step("8a) Partner auth with retired key → expect 401"))
    probe_partner_auth(
        client,
        terminal_id,
        previous_api_key,
        expect_unauthorized=True,
    )

    pause()
    print(cli_style.step("8b) Partner auth with active key → still accepted"))
    probe_partner_auth(client, terminal_id, api_key, expect_unauthorized=False)

    pause()
    print(cli_style.step("9) GET after retire — retiring must be gone"))
    _, get_final = client.request(
        "GET",
        "/api/v1/merchant/terminal",
        headers=auth,
        expect_statuses={200},
    )
    final_list = data_of(get_final)
    if not isinstance(final_list, list) or len(final_list) != 1:
        raise SystemExit("expected exactly one terminal after retire")

    final = final_list[0]
    if not isinstance(final, dict):
        raise SystemExit("GET final item must be an object")

    if final.get("retiring") is not None:
        raise SystemExit("GET after retire still has retiring")

    if final.get("active_credential") is None:
        raise SystemExit("GET after retire missing active_credential")

    print(cli_style.ok("merchant terminal lifecycle smoke OK"))
    cli_style.print_credentials_block(
        terminal_id=terminal_id,
        api_key=api_key,
        http_status=final_issue_status,
        extra={"external_merchant_id": external_merchant_id},
    )
    print(
        cli_style.warn(
            "If checkout-flow still points at Damz, update BNPL_TERMINAL_ID / "
            "BNPL_API_KEY there to the values above."
        )
    )


if __name__ == "__main__":
    main()
