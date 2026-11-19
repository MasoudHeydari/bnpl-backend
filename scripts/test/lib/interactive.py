"""Interactive TTY menus for BNPL smoke scripts (stdlib only).

Arrow Up / Arrow Down move the highlight; Enter confirms.
Selected option is bold/bright; others stay dim.
Falls back to numbered input when stdin is not a TTY.
"""

from __future__ import annotations

import sys
import termios
import tty
from typing import Sequence, TypeVar

from lib import cli_style

T = TypeVar("T")

_BOLD = "\033[1m"
_DIM = "\033[2m"
_BRIGHT_CYAN = "\033[96m"
_BRIGHT_GREEN = "\033[92m"
_GRAY = "\033[90m"


def select(title: str, options: Sequence[tuple[str, T]]) -> T:
    """Prompt for one of ``options`` as ``(label, value)`` pairs.

    Raises SystemExit on EOF / empty options / abort.
    """
    if not options:
        raise SystemExit("no options to select")

    if not _stdin_is_tty():
        return _select_numbered(title, options)

    return _select_arrows(title, options)


def _stdin_is_tty() -> bool:
    return hasattr(sys.stdin, "isatty") and sys.stdin.isatty()


def _select_numbered(title: str, options: Sequence[tuple[str, T]]) -> T:
    print(cli_style.step(f"\n{title}"))
    valid: dict[str, T] = {}
    for index, (label, value) in enumerate(options, start=1):
        key = str(index)
        valid[key] = value
        print(cli_style.muted(f"  {key}) {label}"))

    keys = " or ".join(sorted(valid, key=int))
    while True:
        try:
            choice = input(cli_style.step(f"Enter {keys} (Ctrl-C to abort): ")).strip()
        except EOFError:
            raise SystemExit("\nno choice entered") from None

        if choice in valid:
            return valid[choice]

        print(cli_style.warn(f"invalid choice; enter {keys}."))


def _select_arrows(title: str, options: Sequence[tuple[str, T]]) -> T:
    print(cli_style.step(f"\n{title}"))
    print(cli_style.muted("  ↑/↓ move · Enter select · Ctrl-C abort"))

    index = 0
    count = len(options)
    _draw_menu(options, index, first=True)

    fd = sys.stdin.fileno()
    old = termios.tcgetattr(fd)
    try:
        tty.setcbreak(fd)
        while True:
            key = _read_key()
            if key in {"\r", "\n"}:
                _finalize_menu(options, index)
                return options[index][1]

            if key == "up":
                index = (index - 1) % count
                _draw_menu(options, index, first=False)

            elif key == "down":
                index = (index + 1) % count
                _draw_menu(options, index, first=False)

            elif key in {"\x03", "\x1b"}:  # Ctrl-C or Esc
                raise KeyboardInterrupt
    finally:
        termios.tcsetattr(fd, termios.TCSADRAIN, old)


def _draw_menu(
    options: Sequence[tuple[str, T]],
    index: int,
    *,
    first: bool,
) -> None:
    if not first:
        # Move cursor to the first option line and rewrite in place.
        sys.stdout.write(f"\033[{len(options)}A")

    for i, (label, _value) in enumerate(options):
        sys.stdout.write("\033[2K")  # clear line
        if i == index:
            line = cli_style.paint(f"❯ {label}", _BOLD, _BRIGHT_CYAN)
            sys.stdout.write(f"  {line}\n")
        else:
            line = cli_style.paint(f"  {label}", _DIM, _GRAY)
            sys.stdout.write(f"  {line}\n")

    sys.stdout.flush()


def _finalize_menu(options: Sequence[tuple[str, T]], index: int) -> None:
    """Leave the menu with only the chosen row highlighted."""
    sys.stdout.write(f"\033[{len(options)}A")
    for i, (label, _value) in enumerate(options):
        sys.stdout.write("\033[2K")
        if i == index:
            line = cli_style.paint(f"✓ {label}", _BOLD, _BRIGHT_GREEN)
            sys.stdout.write(f"  {line}\n")
        else:
            line = cli_style.paint(f"  {label}", _DIM, _GRAY)
            sys.stdout.write(f"  {line}\n")

    sys.stdout.flush()


def _read_key() -> str:
    ch = sys.stdin.read(1)
    if ch != "\x1b":
        return ch

    # CSI sequences: ESC [ A/B (arrows), or lone ESC.
    nxt = sys.stdin.read(1)
    if nxt != "[":
        return "\x1b"

    code = sys.stdin.read(1)
    if code == "A":
        return "up"

    if code == "B":
        return "down"

    return "\x1b"
