"""Wall 墙 — a Cybersyn-style TUI wall for every running Claude Code session."""

from __future__ import annotations

import textwrap
import time
from pathlib import Path

from textual import events
from textual.app import App, ComposeResult
from textual.binding import Binding
from textual.containers import Grid, Vertical, VerticalScroll
from textual.content import Content
from textual.message import Message
from textual.reactive import reactive
from textual.screen import ModalScreen
from textual.widgets import Footer, Header, Static, TextArea

from . import hypr, kitty, sources

REFRESH_SECONDS = 2.0
MAX_EVENTS_PER_CARD = 40
TRANSCRIPT_TAIL_LINES = 300

KIND_COLOR = {
    "thinking": "magenta",
    "tool_use": "cyan",
    "tool_result": "yellow",
    "text": "green",
    "user_prompt": "bright_white",
    "unknown": "white",
}
KIND_GLYPH = {
    "thinking": "✦",
    "tool_use": "▸",
    "tool_result": "◂",
    "text": "◆",
    "user_prompt": "›",
    "unknown": "·",
}
KIND_LABEL = {
    "thinking": "think",
    "tool_use": "tool ",
    "tool_result": "     ",
    "text": "text ",
    "user_prompt": "you  ",
    "unknown": "???  ",
}


def _elapsed(ts: float) -> str:
    if ts <= 0:
        return "—"
    delta = max(0, int(time.time() - ts))
    if delta < 60:
        return f"{delta}s"
    if delta < 3600:
        return f"{delta // 60}m{delta % 60:02d}s"
    return f"{delta // 3600}h{(delta % 3600) // 60:02d}m"


def _project_name(cwd: str) -> str:
    return Path(cwd).name or cwd


class Card(VerticalScroll):
    """One claude. One card — scrollable history inside."""

    can_focus = True
    # Don't inherit VerticalScroll's arrow-key scroll bindings; the app owns
    # arrow keys for grid navigation. Mouse wheel + pgup/pgdn scroll instead.
    INHERIT_BINDINGS = False

    BINDINGS = [
        Binding("pageup", "page_up", "Page up", show=False),
        Binding("pagedown", "page_down", "Page down", show=False),
        Binding("home", "scroll_home", "Top", show=False),
        Binding("end", "scroll_end", "Bottom", show=False),
    ]

    DEFAULT_CSS = """
    Card {
        border: round $primary;
        padding: 1 2;
        height: 100%;
        scrollbar-size-vertical: 1;
    }
    Card > Static { height: auto; width: 100%; }
    Card.-waiting { border: thick $success; }
    /* Focus wins over every other card state. */
    Card:focus, Card.-waiting:focus {
        border: double $warning;
        background: $warning 10%;
    }
    """

    def __init__(self, session: sources.Session, window: hypr.Window | None) -> None:
        super().__init__()
        self.session = session
        self.window = window

    def compose(self) -> ComposeResult:
        yield Static(id="card-body")

    def on_mount(self) -> None:
        self._apply_state_classes()
        self._rebuild()
        # Start pinned to the newest event.
        self.call_after_refresh(lambda: self.scroll_end(animate=False))

    def on_resize(self, event: events.Resize) -> None:
        # Width drives summary wrapping, so rebuild when the card resizes.
        sticky = self._at_bottom()
        self._rebuild()
        if sticky:
            self.call_after_refresh(lambda: self.scroll_end(animate=False))

    def refresh_card(
        self, session: sources.Session, window: hypr.Window | None
    ) -> None:
        self.session = session
        self.window = window
        self._apply_state_classes()
        # Only snap to bottom on refresh if the user was already at the
        # bottom — don't yank them out of history they're reading.
        sticky = self._at_bottom()
        self._rebuild()
        if sticky:
            self.call_after_refresh(lambda: self.scroll_end(animate=False))

    def _at_bottom(self) -> bool:
        return self.scroll_y >= max(0, self.max_scroll_y - 1)

    def _rebuild(self) -> None:
        try:
            body = self.query_one("#card-body", Static)
        except Exception:
            return
        body.update(self._build_content())

    def _apply_state_classes(self) -> None:
        self.set_class(self.session.is_waiting, "-waiting")

    # Row layout (monospace):
    #   "  ▸  tool   Bash"                  ← prefix + optional tool name
    #   "             Try running via uv"   ← summary, hanging-indented
    #
    # Prefix is 2 sp + 1 glyph + 2 sp + 5-char label + 2 sp = 12 cols.
    _ROW_INDENT = "  "
    _LABEL_GAP = "  "
    _SUMMARY_GAP = "  "
    _HANG = " " * (len(_ROW_INDENT) + 1 + len(_LABEL_GAP) + 5 + len(_SUMMARY_GAP))

    def _build_content(self) -> Content:
        s = self.session
        ws = f"WS{self.window.workspace}" if self.window else "WS?"
        project = _project_name(s.cwd)
        elapsed = _elapsed(s.last_event_ts)

        # Header row.
        header_parts: list[Content] = [
            Content.from_markup(
                "[bold magenta]$ws[/]  [bold]$project[/]",
                ws=ws, project=project,
            )
        ]
        if s.is_waiting:
            header_parts.append(
                Content.from_markup("  [bold black on green] WAITING [/]")
            )
        elif self.window and self.window.wclass != "kitty":
            header_parts.append(
                Content.from_markup(
                    "  [dim]\\[$wclass][/]", wclass=self.window.wclass,
                )
            )
        header_parts.append(
            Content.from_markup("  [dim]· $elapsed ago[/]", elapsed=elapsed)
        )
        header = Content("").join(header_parts)

        if not s.events:
            return header + Content("\n\n") + Content.from_markup(
                "[dim](no recent activity)[/]"
            )

        # Available width for the summary column, minus the hanging indent.
        # Reserve 2 cols for the scrollbar + safety so the terminal doesn't
        # re-wrap our lines at column 0 when content overflows.
        inner_w = max(20, (self.content_size.width or 60) - 2)
        summary_w = max(16, inner_w - len(self._HANG))

        blocks: list[Content] = [header]
        for ev in s.events[-MAX_EVENTS_PER_CARD:]:
            blocks.append(Content(""))  # breathing room between events
            blocks.extend(self._event_block(ev, summary_w))

        return Content("\n").join(blocks)

    def _event_block(self, ev: sources.Event, summary_w: int) -> list[Content]:
        color = KIND_COLOR.get(ev.kind, "white")
        glyph = KIND_GLYPH.get(ev.kind, "·")
        label = KIND_LABEL.get(ev.kind, ev.kind[:5].ljust(5))

        # color/glyph/label come from our own dicts, so inlining them into the
        # markup template is safe — and $var substitution doesn't expand inside
        # tag brackets, so we can't use it for the color.
        head = Content.from_markup(
            f"{self._ROW_INDENT}[{color}]{glyph}[/]"
            f"{self._LABEL_GAP}[dim]{label}[/]{self._SUMMARY_GAP}"
        )

        summary_lines = self._wrap_summary(ev.summary, summary_w)

        if ev.tool_name:
            first = head + Content.from_markup(
                f"[bold {color}]$tool[/]", tool=ev.tool_name,
            )
            rest = [Content(self._HANG) + s for s in summary_lines]
            return [first, *rest]

        if not summary_lines:
            return [head + Content.from_markup("[dim](empty)[/]")]

        first = head + summary_lines[0]
        rest = [Content(self._HANG) + s for s in summary_lines[1:]]
        return [first, *rest]

    @staticmethod
    def _wrap_summary(text: str, width: int) -> list[Content]:
        if not text:
            return []
        out: list[Content] = []
        for para in text.split("\n"):
            if not para:
                out.append(Content(""))
                continue
            wrapped = textwrap.wrap(
                para,
                width=width,
                break_long_words=True,
                break_on_hyphens=False,
                replace_whitespace=False,
                drop_whitespace=False,
            ) or [para]
            out.extend(Content(line) for line in wrapped)
        return out

    def on_click(self) -> None:
        self._jump()

    def _jump(self) -> None:
        if self.window:
            hypr.focus(self.window.address)

    def can_send_text(self) -> bool:
        return self.window is not None and self.window.wclass == "kitty"


class PromptArea(TextArea):
    """TextArea that submits on Enter (Shift+Enter for newline)."""

    class Submitted(Message):
        def __init__(self, value: str) -> None:
            super().__init__()
            self.value = value

    def _on_key(self, event: events.Key) -> None:
        # Plain Enter submits; shift+enter / ctrl+j fall through as newline.
        if event.key == "enter":
            event.prevent_default()
            event.stop()
            self.post_message(self.Submitted(self.text))


class PromptModal(ModalScreen[str | None]):
    """Send text into a kitty-hosted claude."""

    DEFAULT_CSS = """
    PromptModal {
        align: center middle;
    }
    #prompt-box {
        width: 80%;
        max-width: 120;
        height: auto;
        border: thick $accent;
        padding: 1 2;
        background: $surface;
    }
    #prompt-title {
        margin-bottom: 1;
        color: $accent;
    }
    #prompt-hint {
        margin-top: 1;
        color: $text-muted;
        text-style: italic;
    }
    PromptArea {
        height: auto;
        max-height: 20;
        min-height: 3;
        border: none;
        padding: 0;
        background: $surface;
    }
    """

    BINDINGS = [Binding("escape", "cancel", "Cancel")]

    def __init__(self, target_label: str) -> None:
        super().__init__()
        self.target_label = target_label

    def compose(self) -> ComposeResult:
        yield Vertical(
            Static(
                f"[bold]→[/] send to [cyan]{self.target_label}[/]",
                id="prompt-title",
            ),
            PromptArea(soft_wrap=True, show_line_numbers=False),
            Static(
                "Enter to send · Shift+Enter for newline · Esc to cancel",
                id="prompt-hint",
            ),
            id="prompt-box",
        )

    def on_mount(self) -> None:
        self.query_one(PromptArea).focus()

    def on_prompt_area_submitted(self, event: PromptArea.Submitted) -> None:
        self.dismiss(event.value)

    def action_cancel(self) -> None:
        self.dismiss(None)


class Wall(App[None]):
    CSS = """
    #grid {
        grid-size: 3;
        grid-gutter: 1 2;
        padding: 1 2;
    }
    #empty {
        padding: 4 6;
        text-style: italic;
        color: $text-muted;
    }
    """

    BINDINGS = [
        Binding("r", "refresh_now", "Refresh"),
        Binding("enter", "jump", "Jump"),
        Binding("i", "prompt", "Send prompt"),
        Binding("left,h", "move(-1)", "←", show=False),
        Binding("right,l", "move(1)", "→", show=False),
        Binding("up,k", "move_row(-1)", "↑", show=False),
        Binding("down,j", "move_row(1)", "↓", show=False),
        Binding("q", "quit", "Quit"),
    ]

    GRID_COLUMNS = 3

    sessions: reactive[list[sources.Session]] = reactive([])

    def compose(self) -> ComposeResult:
        yield Header(show_clock=True)
        yield Vertical(Grid(id="grid"), Static("scanning…", id="empty"))
        yield Footer()

    def on_mount(self) -> None:
        self.title = "Wall 墙"
        self.sub_title = "claude wall"
        self.set_interval(REFRESH_SECONDS, self._scan)
        self._scan()

    def _scan(self) -> None:
        windows = hypr.clients()
        sessions = sources.discover(event_limit=MAX_EVENTS_PER_CARD)

        grid = self.query_one("#grid", Grid)
        empty = self.query_one("#empty", Static)

        if not sessions:
            grid.display = False
            empty.display = True
            empty.update("[dim]No active Claude sessions detected.[/]")
            return

        empty.display = False
        grid.display = True

        paired = [(s, hypr.window_for_pid(s.pid, windows)) for s in sessions]
        # Stable read order: by workspace ascending, then by project name.
        # Sessions whose window we couldn't resolve go last.
        paired.sort(
            key=lambda pw: (
                pw[1].workspace if pw[1] else 10**9,
                _project_name(pw[0].cwd),
                pw[0].pid,
            )
        )

        existing = list(grid.query(Card))
        for i, (sess, win) in enumerate(paired):
            if i < len(existing):
                existing[i].refresh_card(sess, win)
            else:
                grid.mount(Card(sess, win))
        for extra in existing[len(paired) :]:
            extra.remove()

        # If nothing card-ish is focused, pin focus on the first card so arrow
        # keys work straight away without needing Tab first. Skip while a modal
        # is up, otherwise we yank focus out of the prompt input mid-typing.
        if self.screen is self and not isinstance(self.focused, Card):
            first = grid.query(Card).first()
            if first is not None:
                first.focus()

    # ── actions ────────────────────────────────────────────────────────────

    def _cards(self) -> list[Card]:
        try:
            grid = self.query_one("#grid", Grid)
        except Exception:
            return []
        return list(grid.query(Card))

    def _focus_index(self, cards: list[Card]) -> int:
        focused = self.focused
        if isinstance(focused, Card) and focused in cards:
            return cards.index(focused)
        return -1

    def action_move(self, delta: int) -> None:
        cards = self._cards()
        if not cards:
            return
        i = self._focus_index(cards)
        if i < 0:
            cards[0].focus()
            return
        cards[max(0, min(len(cards) - 1, i + delta))].focus()

    def action_move_row(self, delta: int) -> None:
        cards = self._cards()
        if not cards:
            return
        i = self._focus_index(cards)
        if i < 0:
            cards[0].focus()
            return
        target = i + delta * self.GRID_COLUMNS
        if 0 <= target < len(cards):
            cards[target].focus()

    def action_refresh_now(self) -> None:
        self._scan()

    def action_jump(self) -> None:
        focused = self.focused
        if isinstance(focused, Card):
            focused._jump()

    def action_prompt(self) -> None:
        focused = self.focused
        if not isinstance(focused, Card):
            self.notify("Focus a card first.", severity="warning", timeout=2)
            return
        if not focused.can_send_text():
            wclass = focused.window.wclass if focused.window else "?"
            self.notify(
                f"Can't drive [{wclass}] from here — only kitty-hosted claudes.",
                severity="warning",
                timeout=3,
            )
            return
        project = _project_name(focused.session.cwd)
        label = f"WS{focused.window.workspace} {project}"
        claude_pid = focused.session.pid

        def _after(text: str | None) -> None:
            if not text:
                return
            ok, err = kitty.send_text(claude_pid, text)
            if ok:
                self.notify(f"Sent to {label}", timeout=2)
            else:
                self.notify(f"Send failed: {err}", severity="error", timeout=5)

        self.push_screen(PromptModal(label), _after)


def main() -> None:
    Wall().run()


if __name__ == "__main__":
    main()
