# opsroom

A Cybersyn-style TUI wall for every running Claude Code session.

Scans `~/.claude/projects/*/*.jsonl` and `hyprctl clients` to show, in one grid:
the workspace, project, elapsed time, and current activity (thinking / tool use /
last response) of every live `claude` process on the machine. Click a card or
hit **Enter** to jump Hyprland to that window.

## Run

```bash
uv run opsroom
```

Or install once:

```bash
uv tool install --editable .
opsroom
```

## Keys

| key     | action                              |
|---------|-------------------------------------|
| r       | force refresh                       |
| enter   | focus the Hyprland window behind the selected card |
| q       | quit                                |

## Requires

- Hyprland (`hyprctl`)
- `pgrep` (procps)
- Python ≥ 3.12, Textual ≥ 0.85
