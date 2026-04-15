"""Discover running Claude sessions and summarize their latest activity."""

from __future__ import annotations

import json
import os
import time
from dataclasses import dataclass, field
from pathlib import Path

PROJECTS_DIR = Path.home() / ".claude" / "projects"
# Treat a transcript as "active" if it was appended to in the last N seconds.
ACTIVE_WINDOW_S = 120
# If the most recent event is an assistant final text, claude is probably
# waiting for the user once this many seconds have passed with no new events.
WAITING_AFTER_S = 3.0
# Cap on transcript tail lines scanned per session.
TRANSCRIPT_TAIL_LINES = 400


@dataclass
class Event:
    ts: float  # unix seconds
    kind: str  # "thinking" | "tool_use" | "tool_result" | "text" | "user_prompt"
    summary: str  # short human-readable snippet
    tool_name: str | None = None


@dataclass
class Session:
    pid: int
    cwd: str
    project_slug: str
    transcript: Path
    events: list[Event] = field(default_factory=list)  # oldest → newest
    is_waiting: bool = False

    @property
    def last_event_ts(self) -> float:
        return self.events[-1].ts if self.events else 0.0


def _ppid_of(pid: int) -> int | None:
    try:
        with open(f"/proc/{pid}/status") as f:
            for line in f:
                if line.startswith("PPid:"):
                    return int(line.split()[1])
    except OSError:
        return None
    return None


def _comm_of(pid: int) -> str | None:
    try:
        with open(f"/proc/{pid}/comm") as f:
            return f.read().strip()
    except OSError:
        return None


def _claude_pids() -> list[int]:
    """Top-level `claude` processes only — skip children whose parent is also claude."""
    try:
        out = os.popen("pgrep -x claude").read()
    except Exception:
        return []
    all_pids = [int(x) for x in out.split() if x.strip().isdigit()]
    result = []
    for pid in all_pids:
        ppid = _ppid_of(pid)
        if ppid and _comm_of(ppid) == "claude":
            continue
        result.append(pid)
    return result


def _cwd_of(pid: int) -> str | None:
    try:
        return os.readlink(f"/proc/{pid}/cwd")
    except OSError:
        return None


def _session_id_from_cmdline(pid: int) -> str | None:
    """VSCode spawns claude with `--resume <uuid>`. Extract it when present."""
    try:
        with open(f"/proc/{pid}/cmdline", "rb") as f:
            args = [x.decode("utf-8", "replace") for x in f.read().split(b"\0") if x]
    except OSError:
        return None
    for i, a in enumerate(args):
        if a == "--resume" and i + 1 < len(args):
            return args[i + 1]
    return None


def _slug_for(cwd: str) -> str:
    return cwd.replace("/", "-")


def _tail_lines(path: Path, n: int = 400) -> list[str]:
    """Cheap tail: read last ~512KB and split. Good enough for JSONL streams."""
    try:
        with path.open("rb") as f:
            f.seek(0, os.SEEK_END)
            size = f.tell()
            read_from = max(0, size - 512 * 1024)
            f.seek(read_from)
            chunk = f.read().decode("utf-8", errors="replace")
    except OSError:
        return []
    lines = [line for line in chunk.splitlines() if line.strip()]
    return lines[-n:]


def _parse_ts(s: str) -> float:
    import datetime as dt

    try:
        return dt.datetime.fromisoformat(s.replace("Z", "+00:00")).timestamp()
    except ValueError:
        return 0.0


def _event_from_raw(raw: str) -> Event | None:
    try:
        ev = json.loads(raw)
    except json.JSONDecodeError:
        return None

    ts = _parse_ts(ev.get("timestamp") or "")
    role = ev.get("type")  # "user" | "assistant" | "attachment" | ...
    msg = ev.get("message") or {}
    content = msg.get("content")

    # User prompts arrive with `content` as a plain string.
    if role == "user" and isinstance(content, str) and content.strip():
        return Event(ts=ts, kind="user_prompt", summary=content.strip())

    if not isinstance(content, list) or not content:
        return None
    item = content[0]
    if not isinstance(item, dict):
        return None

    t = item.get("type", "")
    if t == "thinking":
        return Event(ts=ts, kind="thinking", summary=(item.get("thinking") or "").strip())
    if t == "tool_use":
        name = item.get("name") or "?"
        inp = item.get("input") or {}
        desc = (
            inp.get("description")
            or inp.get("command")
            or inp.get("file_path")
            or inp.get("pattern")
            or inp.get("prompt")
            or ""
        )
        desc = desc.strip() if isinstance(desc, str) else ""
        return Event(ts=ts, kind="tool_use", summary=desc, tool_name=name)
    if t == "tool_result":
        raw_c = item.get("content")
        if isinstance(raw_c, list) and raw_c and isinstance(raw_c[0], dict):
            raw_c = raw_c[0].get("text", "")
        # Tool results can be megabytes (whole file contents, big command
        # output). Cap generously so the card still renders, but keep newlines.
        text = str(raw_c or "").strip()
        if len(text) > 4000:
            text = text[:4000] + "\n… (truncated)"
        return Event(ts=ts, kind="tool_result", summary=text)
    if t == "text":
        return Event(ts=ts, kind="text", summary=(item.get("text") or "").strip())
    # Raw user prompt (the "please do X" from the human).
    if role == "user" and not item.get("tool_use_id"):
        text = (
            item.get("text")
            or item.get("content")
            or ""
        )
        if isinstance(text, str) and text.strip():
            return Event(ts=ts, kind="user_prompt", summary=text.strip())
    return None


def _recent_events(lines: list[str], limit: int = 8) -> list[Event]:
    events: list[Event] = []
    for raw in lines:
        ev = _event_from_raw(raw)
        if ev:
            events.append(ev)
    return events[-limit:]


def discover(event_limit: int = 8) -> list[Session]:
    if not PROJECTS_DIR.is_dir():
        return []
    now = time.time()
    pids = _claude_pids()

    # Pass 1: pids that tell us their session id via --resume (VSCode path).
    pid_to_transcript: dict[int, Path] = {}
    claimed: set[str] = set()
    unresolved: list[int] = []
    for pid in pids:
        sid = _session_id_from_cmdline(pid)
        cwd = _cwd_of(pid)
        if not sid or not cwd:
            unresolved.append(pid)
            continue
        candidate = PROJECTS_DIR / _slug_for(cwd) / f"{sid}.jsonl"
        if candidate.is_file():
            pid_to_transcript[pid] = candidate
            claimed.add(str(candidate))
        else:
            unresolved.append(pid)

    # Pass 2: terminal claudes. Assign each the most-recently-modified jsonl
    # in its project dir that hasn't been claimed yet.
    for pid in unresolved:
        cwd = _cwd_of(pid)
        if not cwd:
            continue
        project_dir = PROJECTS_DIR / _slug_for(cwd)
        if not project_dir.is_dir():
            continue
        candidates = sorted(
            (p for p in project_dir.glob("*.jsonl") if str(p) not in claimed),
            key=lambda p: p.stat().st_mtime,
            reverse=True,
        )
        if not candidates:
            continue
        pid_to_transcript[pid] = candidates[0]
        claimed.add(str(candidates[0]))

    sessions: list[Session] = []
    for pid, transcript in pid_to_transcript.items():
        mtime = transcript.stat().st_mtime
        if now - mtime > ACTIVE_WINDOW_S * 10:
            continue
        cwd = _cwd_of(pid) or ""
        lines = _tail_lines(transcript, n=TRANSCRIPT_TAIL_LINES)
        events = _recent_events(lines, limit=event_limit)
        waiting = _is_waiting(events, now)
        sessions.append(
            Session(
                pid=pid,
                cwd=cwd,
                project_slug=_slug_for(cwd),
                transcript=transcript,
                events=events,
                is_waiting=waiting,
            )
        )
    return sessions


def _is_waiting(events: list[Event], now: float) -> bool:
    """A session is 'waiting' if its last event is an assistant text message
    (a final response) and some idle time has passed without further events."""
    if not events:
        return False
    last = events[-1]
    if last.kind != "text":
        return False
    return (now - last.ts) >= WAITING_AFTER_S
