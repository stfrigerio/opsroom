"""Hyprland queries — which window owns each claude process, and jumping to it."""

from __future__ import annotations

import json
import subprocess
from dataclasses import dataclass


@dataclass
class Window:
    address: str
    pid: int
    workspace: int
    title: str
    wclass: str


def clients() -> list[Window]:
    out = subprocess.run(
        ["hyprctl", "clients", "-j"], capture_output=True, text=True, check=True
    ).stdout
    return [
        Window(
            address=c["address"],
            pid=c["pid"],
            workspace=c["workspace"]["id"],
            title=c["title"],
            wclass=c["class"],
        )
        for c in json.loads(out)
    ]


def _ppid(pid: int) -> int | None:
    try:
        with open(f"/proc/{pid}/status") as f:
            for line in f:
                if line.startswith("PPid:"):
                    return int(line.split()[1])
    except FileNotFoundError:
        return None
    return None


def window_for_pid(pid: int, windows: list[Window]) -> Window | None:
    """Walk up the PPID chain until we hit a pid that owns a Hyprland window."""
    win_by_pid = {w.pid: w for w in windows}
    cur: int | None = pid
    while cur and cur > 1:
        if cur in win_by_pid:
            return win_by_pid[cur]
        cur = _ppid(cur)
    return None


def focus(address: str) -> None:
    subprocess.run(
        ["hyprctl", "dispatch", "focuswindow", f"address:{address}"],
        check=False,
    )
