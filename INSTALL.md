# Install

The wall Go binary is wired into the system via a wrapper script kept in
`LinuxConfig/scripts` and a symlink in `~/.local/bin`. This matches the
existing pattern used for `claude-switcher`, `nav`, etc.

## Layout

```
~/Github/wall/go/wall                                # compiled binary
~/Github/LinuxConfig/scripts/wall/wall.sh            # wrapper (exec's the binary)
~/.local/bin/wall           → .../LinuxConfig/scripts/wall/wall.sh
```

`~/.local/bin` is already on `$PATH`, so `wall` launches from anywhere.

## One-time setup (already done)

```bash
mkdir -p ~/Github/LinuxConfig/scripts/wall

cat > ~/Github/LinuxConfig/scripts/wall/wall.sh <<'EOF'
#!/usr/bin/env bash
exec /home/stefano/Github/wall/go/wall "$@"
EOF

chmod +x ~/Github/LinuxConfig/scripts/wall/wall.sh

ln -sf ~/Github/LinuxConfig/scripts/wall/wall.sh \
       ~/.local/bin/wall
```

## Rebuild flow

After editing Go sources:

```bash
cd ~/Github/wall/go
go build -o wall .
```

The wrapper execs the binary by absolute path, so there is no "re-install"
step — the next `wall` invocation runs the freshly built binary.

## Uninstall

```bash
rm ~/.local/bin/wall
rm -rf ~/Github/LinuxConfig/scripts/wall
```
