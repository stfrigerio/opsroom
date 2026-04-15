# Install

The opsroom Go binary is wired into the system via a wrapper script kept in
`LinuxConfig/scripts` and a symlink in `~/.local/bin`. This matches the
existing pattern used for `claude-switcher`, `nav`, etc.

## Layout

```
~/Github/opsroom/go/opsroom                                # compiled binary
~/Github/LinuxConfig/scripts/opsroom/opsroom.sh            # wrapper (exec's the binary)
~/.local/bin/opsroom           → .../LinuxConfig/scripts/opsroom/opsroom.sh
```

`~/.local/bin` is already on `$PATH`, so `opsroom` launches from anywhere.

## One-time setup (already done)

```bash
mkdir -p ~/Github/LinuxConfig/scripts/opsroom

cat > ~/Github/LinuxConfig/scripts/opsroom/opsroom.sh <<'EOF'
#!/usr/bin/env bash
exec /home/stefano/Github/opsroom/go/opsroom "$@"
EOF

chmod +x ~/Github/LinuxConfig/scripts/opsroom/opsroom.sh

ln -sf ~/Github/LinuxConfig/scripts/opsroom/opsroom.sh \
       ~/.local/bin/opsroom
```

## Rebuild flow

After editing Go sources:

```bash
cd ~/Github/opsroom/go
go build -o opsroom .
```

The wrapper execs the binary by absolute path, so there is no "re-install"
step — the next `opsroom` invocation runs the freshly built binary.

## Uninstall

```bash
rm ~/.local/bin/opsroom
rm -rf ~/Github/LinuxConfig/scripts/opsroom
```
