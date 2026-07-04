#!/usr/bin/env python3
"""Interactive hosted checkout happy path against a live BNPL API.

Flow:
  1. Partner create checkout session
  2. Hosted start
  3. Hosted request OTP
  4. Wait for OTP typed in the CLI
  5. Hosted confirm

The public token returned as data.checkout_session_id (cs_...) is reused for
all hosted steps. Partner create needs X-Terminal-Id / X-API-Key; hosted token
routes are unauthenticated.

Config (env, optional repo .env):
  BNPL_API_BASE              default https://api.bnpl.test:8443
  BNPL_TERMINAL_ID           required
  BNPL_API_KEY               required
  BNPL_ONLINE_STORE_ID       required
  BNPL_CUSTOMER_PHONE        default +989195474883
  BNPL_RETURN_DESTINATION    default default
  BNPL_AMOUNT_MINOR          default 15000000
  BNPL_CURRENCY              default IRR
  BNPL_CUSTOMER_REFERENCE    optional
  BNPL_CA_CERT               optional PEM CA path (e.g. assets/local-development/certs/ca.crt)
  BNPL_SSL_INSECURE          set 1 to skip TLS verify (local only)
"""

from __future__ import annotations

import json
import os
import ssl
import sys
import time
import uuid
import urllib.error
import urllib.request
from pathlib import Path
from typing import Any


REPO_ROOT = Path(__file__).resolve().parents[2]


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


def env(name: str, default: str | None = None) -> str:
    value = os.environ.get(name, default)
    if value is None or value == "":
        raise SystemExit(f"missing required env: {name}")

    return value


def optional_env(name: str, default: str = "") -> str:
    return os.environ.get(name, default).strip()


def candidate_ca_files() -> list[Path]:
    candidates: list[Path] = []

    explicit = optional_env("BNPL_CA_CERT")
    if explicit:
        candidates.append(Path(explicit).expanduser())

    # Local Envoy for api.bnpl.test is typically issued by mkcert.
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


class APIClient:
    def __init__(self, base_url: str, ssl_context: ssl.SSLContext | None) -> None:
        self.base_url = base_url.rstrip("/")
        self.ssl_context = ssl_context

    def request(
        self,
        method: str,
        path: str,
        *,
        body: dict[str, Any] | None = None,
        headers: dict[str, str] | None = None,
    ) -> dict[str, Any]:
        url = f"{self.base_url}{path}"
        payload = None if body is None else json.dumps(body).encode("utf-8")
        req_headers = {
            "Accept": "application/json",
            "X-Correlation-Id": str(uuid.uuid4()),
            "X-Request-Id": str(uuid.uuid4()),
            "X-Trace-Id": str(uuid.uuid4()),
        }
        if body is not None:
            req_headers["Content-Type"] = "application/json"

        if headers:
            req_headers.update(headers)

        request = urllib.request.Request(url, data=payload, headers=req_headers, method=method)
        try:
            with urllib.request.urlopen(request, context=self.ssl_context, timeout=60) as resp:
                raw = resp.read().decode("utf-8")
                status = resp.status
        except urllib.error.HTTPError as err:
            raw = err.read().decode("utf-8", errors="replace")
            print(f"\nHTTP {err.code} {method} {url}\n{raw}\n", file=sys.stderr)
            raise SystemExit(1) from err
        except urllib.error.URLError as err:
            print(f"\nrequest failed: {method} {url}: {err}\n", file=sys.stderr)
            raise SystemExit(1) from err

        try:
            parsed = json.loads(raw) if raw else {}
        except json.JSONDecodeError:
            print(f"\nnon-JSON response ({status}) {method} {url}\n{raw}\n", file=sys.stderr)
            raise SystemExit(1)

        print(f"\n==> {method} {url} -> {status}")
        print(json.dumps(parsed, ensure_ascii=False, indent=2))
        return parsed


def data_of(response: dict[str, Any]) -> dict[str, Any]:
    data = response.get("data")
    if not isinstance(data, dict):
        raise SystemExit("response missing data object")

    return data


REQUEST_DELAY_SECONDS = 1


def pause(seconds: float = REQUEST_DELAY_SECONDS) -> None:
    print(f"\nwaiting {seconds:g}s before next request...")
    time.sleep(seconds)


def prompt_otp() -> str:
    while True:
        try:
            code = input("\nEnter OTP from SMS (or Ctrl-C to abort): ").strip()
        except EOFError:
            raise SystemExit("\nno OTP entered") from None

        if code:
            return code

        print("OTP is empty; try again.")


def main() -> None:
    load_dotenv(REPO_ROOT / ".env")

    base_url = optional_env("BNPL_API_BASE", "https://api.bnpl.test:8443")
    terminal_id = env("BNPL_TERMINAL_ID")
    api_key = env("BNPL_API_KEY")
    external_online_store_id = optional_env("BNPL_EXTERNAL_ONLINE_STORE_ID") or optional_env(
        "BNPL_ONLINE_STORE_ID"
    )
    if not external_online_store_id:
        raise SystemExit(
            "set BNPL_EXTERNAL_ONLINE_STORE_ID (or legacy BNPL_ONLINE_STORE_ID)"
        )

    customer_phone = optional_env("BNPL_CUSTOMER_PHONE", "+989195474883")
    return_destination = optional_env("BNPL_RETURN_DESTINATION", "default")
    amount_minor = int(optional_env("BNPL_AMOUNT_MINOR", "15000000"))
    currency = optional_env("BNPL_CURRENCY", "IRR")
    customer_reference = optional_env("BNPL_CUSTOMER_REFERENCE")

    stamp = time.strftime("%Y%m%d%H%M%S")
    merchant_order_id = optional_env("BNPL_MERCHANT_ORDER_ID", f"ORD-CLI-{stamp}")
    idempotency_key = optional_env("BNPL_IDEMPOTENCY_KEY", f"{merchant_order_id}-ATTEMPT-1")

    partner_headers = {
        "X-Terminal-Id": terminal_id,
        "X-API-Key": api_key,
    }

    client = APIClient(base_url, build_ssl_context())

    create_body: dict[str, Any] = {
        "external_online_store_id": external_online_store_id,
        "merchant_order_id": merchant_order_id,
        "idempotency_key": idempotency_key,
        "customer_phone": customer_phone,
        "return_destination": return_destination,
        "requested_amount_minor": amount_minor,
        "currency": currency,
    }
    if customer_reference:
        create_body["customer_reference"] = customer_reference

    print("1) Create partner checkout session")
    create_resp = client.request(
        "POST",
        "/api/v1/partner/checkout-sessions",
        body=create_body,
        headers=partner_headers,
    )
    create_data = data_of(create_resp)
    token = str(create_data.get("checkout_session_id") or "").strip()
    if not token.startswith("cs_"):
        raise SystemExit(
            "create response did not return public checkout_session_id (cs_...); "
            f"got {create_data.get('checkout_session_id')!r}"
        )

    print(f"\nusing checkout_session_id/token from create: {token}")

    pause()
    print("\n2) Hosted start")
    # First START may omit evaluation_generation when offer is deferred (on_start).
    start_resp = client.request(
        "POST",
        f"/api/v1/checkout-sessions/{token}/start",
        body={},
    )
    start_data = data_of(start_resp)
    evaluation_generation = int(start_data.get("evaluation_generation") or 0)
    if evaluation_generation < 1:
        raise SystemExit(
            "start response missing evaluation_generation; cannot continue to OTP/confirm"
        )

    print(f"\nusing evaluation_generation from start: {evaluation_generation}")

    pause()
    print("\n3) Request OTP")
    client.request(
        "POST",
        f"/api/v1/checkout-sessions/{token}/otp",
        body={"evaluation_generation": evaluation_generation},
    )

    otp = prompt_otp()

    pause()
    print("\n4) Confirm")
    confirm_resp = client.request(
        "POST",
        f"/api/v1/checkout-sessions/{token}/confirm",
        body={
            "evaluation_generation": evaluation_generation,
            "otp": otp,
        },
    )
    confirm_data = data_of(confirm_resp)

    print("\nDone.")
    print(
        json.dumps(
            {
                "checkout_session_id": token,
                "status": confirm_data.get("status"),
                "reconfirm_required": confirm_data.get("reconfirm_required"),
                "payment_required": confirm_data.get("payment_required"),
                "redirect_url": confirm_data.get("redirect_url"),
                "contract": confirm_data.get("contract"),
            },
            ensure_ascii=False,
            indent=2,
        )
    )


if __name__ == "__main__":
    try:
        main()
    except KeyboardInterrupt:
        print("\naborted", file=sys.stderr)
        raise SystemExit(130) from None
