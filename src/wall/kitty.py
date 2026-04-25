"""Drive a remote kitty terminal via `kitty @`.

Requires these lines in kitty.conf (one-time setup):
    allow_remote_control yes
    listen_on unix:${XDG_RUNTIME_DIR}/kitty-$KITTY_PID

Each kitty exports `KITTY_LISTEN_ON` into its child process environment, so we
can recover the right socket path from the claude process itself.
"""

from __future__ import annotations

import os
import shutil
import subprocess
import time


def available() -> bool:
    return shutil.which("kitty") is not None


def _env_of(pid: int) -> dict[str, str]:
    try:
        with open(f"/proc/{pid}/environ", "rb") as f:
            raw = f.read()
    except OSError:
        return {}
    env: dict[str, str] = {}
    for entry in raw.split(b"\0"):
        if not entry or b"=" not in entry:
            continue
        k, _, v = entry.partition(b"=")
        env[k.decode("utf-8", "replace")] = v.decode("utf-8", "replace")
    return env


def listen_socket_for(claude_pid: int) -> str | None:
    """Look up KITTY_LISTEN_ON inherited from the kitty ancestor."""
    return _env_of(claude_pid).get("KITTY_LISTEN_ON")


def send_text(claude_pid: int, text: str, with_enter: bool = True) -> tuple[bool, str]:
    """Send `text` into the kitty that hosts the given claude pid.

    Returns (ok, err_msg).
    """
    if not available():
        return False, "kitty not on PATH"
    socket = listen_socket_for(claude_pid)
    if not socket:
        return False, (
            "no KITTY_LISTEN_ON in env — add `allow_remote_control yes` and "
            "`listen_on unix:${XDG_RUNTIME_DIR}/kitty-$KITTY_PID` to kitty.conf, "
            "then restart the kitty window."
        )
    # Send text and Enter as two separate calls. Claude Code's TUI batches
    # rapidly-arriving input as a paste, which turns a trailing \r into a
    # literal newline in the prompt instead of a submit. A short gap lets it
    # close the paste before Enter lands.
    env = {**os.environ}
    proc = subprocess.run(
        ["kitty", "@", "--to", socket, "send-text", text],
        capture_output=True, text=True, env=env,
    )
    if proc.returncode != 0:
        err = (proc.stderr.strip() or "kitty @ send-text failed").splitlines()[0]
        return False, err
    if with_enter:
        time.sleep(0.08)
        proc = subprocess.run(
            ["kitty", "@", "--to", socket, "send-text", "\r"],
            capture_output=True, text=True, env=env,
        )
        if proc.returncode != 0:
            err = (proc.stderr.strip() or "kitty @ send-text failed").splitlines()[0]
            return False, err
    return True, ""
