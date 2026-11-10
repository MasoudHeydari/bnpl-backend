"""Shared HTTP client for BNPL API smoke scripts."""

from __future__ import annotations

import json
import ssl
import sys
import uuid
import urllib.error
import urllib.request
from typing import Any

from . import cli_style


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
        expect_statuses: set[int] | None = None,
        accept_any_status: bool = False,
    ) -> tuple[int, dict[str, Any]]:
        """Send an HTTP request and print REQUEST/RESPONSE blocks.

        expect_statuses: when set, status must be one of these (unless
        accept_any_status is true).
        accept_any_status: return whatever HTTP status the server sent without
        treating 4xx/5xx as a hard script failure (useful for auth probes).
        """
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

        cli_style.print_request(method, url, req_headers, body)

        request = urllib.request.Request(url, data=payload, headers=req_headers, method=method)
        try:
            with urllib.request.urlopen(request, context=self.ssl_context, timeout=60) as resp:
                raw = resp.read().decode("utf-8")
                status = resp.status
        except urllib.error.HTTPError as err:
            raw = err.read().decode("utf-8", errors="replace")
            status = err.code
            if not accept_any_status and (
                expect_statuses is None or status not in expect_statuses
            ):
                parsed_err = cli_style.try_parse_json(raw)
                cli_style.print_response(
                    method,
                    url,
                    status,
                    parsed_err if parsed_err is not None else raw,
                    file=sys.stderr,
                )
                print(cli_style.err(f"unexpected HTTP {status}"), file=sys.stderr)
                raise SystemExit(1) from err
        except urllib.error.URLError as err:
            print(cli_style.err(f"request failed: {method} {url}: {err}"), file=sys.stderr)
            raise SystemExit(1) from err

        try:
            parsed: Any = json.loads(raw) if raw else {}
        except json.JSONDecodeError:
            cli_style.print_response(method, url, status, raw, file=sys.stderr)
            print(cli_style.err("response is not JSON"), file=sys.stderr)
            raise SystemExit(1)

        if not isinstance(parsed, dict):
            cli_style.print_response(method, url, status, parsed, file=sys.stderr)
            print(cli_style.err("response JSON root must be an object"), file=sys.stderr)
            raise SystemExit(1)

        cli_style.print_response(method, url, status, parsed)

        if (
            not accept_any_status
            and expect_statuses is not None
            and status not in expect_statuses
        ):
            raise SystemExit(f"expected HTTP {sorted(expect_statuses)}, got {status}")

        return status, parsed
