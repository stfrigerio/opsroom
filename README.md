# opsroom

TUI wall for every running Claude Code session.

Scans `/proc` for `claude` processes, matches each to its
`~/.claude/projects/*/*.jsonl` transcript, and renders a live
grid: workspace, project, elapsed time, spinner, and a scrollable
event log (thinking / tool use / text / prompts) per session. Hit
**Enter** to focus the Hyprland window behind the focused card, or
**i** to inject a prompt directly into that claude's kitty pane.

## Build

Needs Go ≥ 1.22.

```bash
cd go
go build -o opsroom .
./opsroom
```

Or drop a symlink on your `$PATH` — see `INSTALL.md`.

## Requires

- Linux (reads `/proc/<pid>/{cwd,stat,cmdline}`)
- Hyprland (`hyprctl` on `$PATH`) for window-to-pid matching and jump
- kitty with remote-control enabled for the `i` inject feature
- `pgrep` (procps)

Without hypr/kitty the grid still renders; only `⏎` and `i` go quiet.

## Keys

**Focus & navigation**

| key                | action                                    |
|--------------------|-------------------------------------------|
| ←↑↓→ / hjkl        | move focus between cards                  |
| mouse motion       | hover-to-focus                            |
| enter              | focus the Hyprland window for this card   |
| i                  | open prompt modal → injects into kitty    |

**Card scrolling**

| key                | action                                    |
|--------------------|-------------------------------------------|
| pgup / pgdn        | scroll the focused event log              |
| home               | jump to top                               |
| end                | snap to bottom + re-pin to new events     |
| wheel up/down      | scroll under cursor                       |

**Pane management**

| key                | action                                    |
|--------------------|-------------------------------------------|
| ctrl+←↑↓→          | swap the focused pane with its neighbour  |
| [ / ] (n / p)      | previous / next page                      |
| ctrl+tab / shift   | previous / next page                      |

**Other**

| key                | action                                    |
|--------------------|-------------------------------------------|
| r                  | force rescan now                          |
| q / ctrl+c         | quit                                      |

## Prompt injection

`i` on a focused card opens a modal. Enter sends — the text is typed
into the kitty window that owns the claude PID via kitty's remote
control socket. Requires `allow_remote_control yes` and a predictable
`listen_on` in your `kitty.conf`. Shift+Enter inserts a newline.

## How it works

- **Discovery** — `pgrep -x claude` finds candidate PIDs; the pid is
  then matched to a `~/.claude/projects/<slug>/<session>.jsonl`
  transcript via `--resume` cmdline flag, process-start-time proximity,
  or newest-in-dir fallback (three passes, in that order).
- **Events** — last ~400 lines of the transcript are parsed into
  thinking / tool-use / text / user-prompt events. Tool results are
  dropped (noise).
- **State** — working = transcript mtime ahead of last visible event
  (hidden blocks streaming); waiting = last event is text ≥ 3s ago.
- **Layout** — up to 3×3 cards per page; more sessions paginate.
  Pane order is stable across scans; `ctrl+arrow` reorders manually.
