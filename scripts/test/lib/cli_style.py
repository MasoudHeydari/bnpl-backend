"""ANSI styling helpers for BNPL smoke scripts (stdlib only).

Respects NO_COLOR / BNPL_NO_COLOR and FORCE_COLOR / BNPL_FORCE_COLOR.
When stdout is not a TTY, colors are off unless FORCE_COLOR is set.
"""

from __future__ import annotations

import json
import os
import sys
from typing import Any, TextIO


_RESET = "\033[0m"
_BOLD = "\033[1m"
_DIM = "\033[2m"

_CYAN = "\033[36m"
_BRIGHT_CYAN = "\033[96m"
_GREEN = "\033[32m"
_BRIGHT_GREEN = "\033[92m"
_YELLOW = "\033[33m"
_BRIGHT_YELLOW = "\033[93m"
_RED = "\033[31m"
_BRIGHT_RED = "\033[91m"
_MAGENTA = "\033[35m"
_BRIGHT_BLUE = "\033[94m"
_WHITE = "\033[97m"
_GRAY = "\033[90m"


def _truthy(name: str) -> bool:
    return os.environ.get(name, "").strip().lower() in {"1", "true", "yes", "on"}


def colors_enabled(stream: TextIO | None = None) -> bool:
    if _truthy("BNPL_NO_COLOR") or _truthy("NO_COLOR"):
        return False

    if _truthy("BNPL_FORCE_COLOR") or _truthy("FORCE_COLOR"):
        return True

    target = stream if stream is not None else sys.stdout
    return hasattr(target, "isatty") and target.isatty()


def paint(text: str, *codes: str, stream: TextIO | None = None) -> str:
    if not codes or not colors_enabled(stream):
        return text

    return f"{''.join(codes)}{text}{_RESET}"


def step(title: str) -> str:
    return paint(f"▶ {title}", _BOLD, _BRIGHT_CYAN)


def ok(message: str) -> str:
    return paint(f"✓ {message}", _BRIGHT_GREEN)


def warn(message: str) -> str:
    return paint(f"! {message}", _BRIGHT_YELLOW)


def err(message: str) -> str:
    return paint(f"✗ {message}", _BRIGHT_RED, stream=sys.stderr)


def muted(message: str) -> str:
    return paint(message, _DIM, _GRAY)


def status_color(status: int) -> str:
    code = str(status)
    if 200 <= status < 300:
        return paint(code, _BOLD, _BRIGHT_GREEN)

    if status == 401 or status == 403:
        return paint(code, _BOLD, _BRIGHT_YELLOW)

    if 400 <= status < 500:
        return paint(code, _BOLD, _YELLOW)

    if status >= 500:
        return paint(code, _BOLD, _BRIGHT_RED)

    return paint(code, _BOLD, _WHITE)


def method_color(method: str) -> str:
    return paint(method, _BOLD, _MAGENTA)


def format_http_line(method: str, url: str, status: int) -> str:
    arrow = paint("==>", _DIM, _CYAN)
    return f"{arrow} {method_color(method)} {url} -> {status_color(status)}"


def _rule(label: str, *, stream: TextIO | None = None) -> str:
    bar = "─" * 12
    return paint(f"{bar} {label} {bar}", _BOLD, _CYAN, stream=stream)


def _field_label(label: str, *, stream: TextIO | None = None) -> str:
    return paint(f"{label}:", _BOLD, _BRIGHT_CYAN, stream=stream)


def print_headers(headers: dict[str, str], *, file: TextIO | None = None) -> None:
    target = file if file is not None else sys.stdout
    if not headers:
        print(muted("(none)"), file=target)
        return

    width = max(len(name) for name in headers)
    for name in sorted(headers, key=str.lower):
        key = paint(f"{name:<{width}}", _BRIGHT_BLUE, stream=target)
        sep = paint(":", _DIM, stream=target)
        value = paint(headers[name], _GREEN, stream=target)
        print(f"  {key} {sep} {value}", file=target)


def print_request(
    method: str,
    url: str,
    headers: dict[str, str],
    body: Any | None,
    *,
    file: TextIO | None = None,
) -> None:
    """Print a readable REQUEST block (headers + body) before the response."""
    target = file if file is not None else sys.stdout
    print(file=target)
    print(_rule("REQUEST", stream=target), file=target)
    print(f"{method_color(method)} {paint(url, _WHITE, stream=target)}", file=target)
    print(_field_label("Headers", stream=target), file=target)
    print_headers(headers, file=target)
    print(_field_label("Body", stream=target), file=target)
    if body is None:
        print(muted("  (no body)"), file=target)
    else:
        # Indent JSON under the Body label for scanability.
        rendered = dumps_colored(body, stream=target)
        for line in rendered.splitlines():
            print(f"  {line}", file=target)

    print(file=target)


def print_response(
    method: str,
    url: str,
    status: int,
    body: Any,
    *,
    file: TextIO | None = None,
) -> None:
    """Print a readable RESPONSE block after a blank line from the request."""
    target = file if file is not None else sys.stdout
    print(_rule("RESPONSE", stream=target), file=target)
    print(format_http_line(method, url, status), file=target)
    print(_field_label("Body", stream=target), file=target)
    if isinstance(body, str):
        for line in body.splitlines() or [body]:
            print(f"  {line}", file=target)
    else:
        rendered = dumps_colored(body, stream=target)
        for line in rendered.splitlines():
            print(f"  {line}", file=target)

    print(file=target)


def print_credentials_block(
    *,
    terminal_id: str,
    api_key: str,
    http_status: int | None = None,
    extra: dict[str, str] | None = None,
) -> None:
    """Final copy-paste block after a successful smoke run."""
    print()
    title = "PARTNER CREDENTIALS"
    if http_status is not None:
        title = f"{title} (HTTP {http_status})"

    print(paint(f"{'═' * 8} {title} {'═' * 8}", _BOLD, _BRIGHT_GREEN))
    print(ok(f"terminal_id = {terminal_id}"))
    print(ok(f"api_key     = {api_key}"))
    if extra:
        for key, value in extra.items():
            print(ok(f"{key} = {value}"))

    print()
    print(muted("export for other scripts:"))
    print(paint(f'export BNPL_TERMINAL_ID="{terminal_id}"', _GREEN))
    print(paint(f'export BNPL_API_KEY="{api_key}"', _GREEN))
    print(paint("═" * (18 + len(title)), _BOLD, _BRIGHT_GREEN))
    print()


def dumps_colored(
    value: Any,
    *,
    indent: int = 2,
    stream: TextIO | None = None,
) -> str:
    """Pretty-print JSON with syntax highlighting when colors are enabled."""
    target = stream if stream is not None else sys.stdout
    if not colors_enabled(target):
        return json.dumps(value, ensure_ascii=False, indent=indent)

    return _render(value, indent=indent, level=0, stream=target)


def _paint(text: str, *codes: str, stream: TextIO) -> str:
    return paint(text, *codes, stream=stream)


def _render(
    value: Any,
    *,
    indent: int,
    level: int,
    stream: TextIO,
) -> str:
    pad = " " * (indent * level)
    inner = " " * (indent * (level + 1))

    if value is None:
        return _paint("null", _GRAY, stream=stream)

    if isinstance(value, bool):
        return _paint("true" if value else "false", _MAGENTA, stream=stream)

    if isinstance(value, (int, float)):
        return _paint(json.dumps(value), _BRIGHT_YELLOW, stream=stream)

    if isinstance(value, str):
        return _paint(json.dumps(value, ensure_ascii=False), _GREEN, stream=stream)

    if isinstance(value, list):
        if not value:
            return _paint("[]", _WHITE, stream=stream)

        parts = [_paint("[", _WHITE, stream=stream)]
        for i, item in enumerate(value):
            comma = "," if i < len(value) - 1 else ""
            parts.append(
                f"{inner}{_render(item, indent=indent, level=level + 1, stream=stream)}{comma}"
            )

        parts.append(f"{pad}{_paint(']', _WHITE, stream=stream)}")
        return "\n".join(parts)

    if isinstance(value, dict):
        if not value:
            return _paint("{}", _WHITE, stream=stream)

        parts = [_paint("{", _WHITE, stream=stream)]
        items = list(value.items())
        for i, (key, item) in enumerate(items):
            comma = "," if i < len(items) - 1 else ""
            key_txt = _paint(
                json.dumps(str(key), ensure_ascii=False),
                _BRIGHT_BLUE,
                stream=stream,
            )
            rendered = _render(item, indent=indent, level=level + 1, stream=stream)
            parts.append(
                f"{inner}{key_txt}{_paint(':', _DIM, stream=stream)} {rendered}{comma}"
            )

        parts.append(f"{pad}{_paint('}', _WHITE, stream=stream)}")
        return "\n".join(parts)

    return _paint(json.dumps(value, ensure_ascii=False), _GREEN, stream=stream)


def print_json(value: Any, *, file: TextIO | None = None) -> None:
    target = file if file is not None else sys.stdout
    print(dumps_colored(value, stream=target), file=target)


def try_parse_json(raw: str) -> Any | None:
    text = raw.strip()
    if not text:
        return None

    try:
        return json.loads(text)
    except json.JSONDecodeError:
        return None
