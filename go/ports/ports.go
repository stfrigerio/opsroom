// Package ports scans /proc/net/tcp{,6} for listening TCP sockets and
// matches each socket inode to the owning PID by walking /proc/*/fd.
// Same technique as ss/netstat — works without root for your own processes
// and also picks up everyone else's listeners (just without PID resolution
// if we can't read their fd dir).
package ports

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// Port — one listening socket.
type Port struct {
	Port  int
	Bind  string // display form: "127.0.0.1", "*", "::1", "[::]"
	Proto string // "tcp" | "tcp6"
	PID   int    // 0 if we couldn't resolve
	Comm  string // /proc/<pid>/comm
	CWD   string // /proc/<pid>/cwd target
	Cmd   string // /proc/<pid>/cmdline, nulls → spaces
}

// Scan — every listening TCP socket on the machine.
func Scan() ([]Port, error) {
	sockets, err := readListeners()
	if err != nil {
		return nil, err
	}
	inodeToPID := buildInodeMap()
	out := make([]Port, 0, len(sockets))
	for _, s := range sockets {
		p := Port{
			Port:  s.port,
			Bind:  s.bind,
			Proto: s.proto,
		}
		if pid, ok := inodeToPID[s.inode]; ok {
			p.PID = pid
			p.Comm = readFile(fmt.Sprintf("/proc/%d/comm", pid))
			if cwd, err := os.Readlink(fmt.Sprintf("/proc/%d/cwd", pid)); err == nil {
				p.CWD = cwd
			}
			p.Cmd = readCmdline(pid)
		}
		out = append(out, p)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Port < out[j].Port })
	return out, nil
}

// ── /proc/net/tcp{,6} parser ──────────────────────────────────────────────

type listener struct {
	proto string
	bind  string
	port  int
	inode uint64
}

func readListeners() ([]listener, error) {
	var out []listener
	v4, err := parseProcNet("/proc/net/tcp", "tcp", false)
	if err != nil {
		return nil, err
	}
	out = append(out, v4...)
	v6, _ := parseProcNet("/proc/net/tcp6", "tcp6", true)
	out = append(out, v6...)
	return out, nil
}

func parseProcNet(path, proto string, ipv6 bool) ([]listener, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var out []listener
	lines := strings.Split(string(data), "\n")
	if len(lines) == 0 {
		return out, nil
	}
	for _, line := range lines[1:] {
		fields := strings.Fields(line)
		if len(fields) < 10 {
			continue
		}
		// fields[1]=local, fields[3]=state (0A=LISTEN), fields[9]=inode
		if fields[3] != "0A" {
			continue
		}
		ip, port, ok := parseHexAddr(fields[1], ipv6)
		if !ok {
			continue
		}
		inode, err := strconv.ParseUint(fields[9], 10, 64)
		if err != nil {
			continue
		}
		out = append(out, listener{
			proto: proto,
			bind:  displayBind(ip, ipv6),
			port:  port,
			inode: inode,
		})
	}
	return out, nil
}

// parseHexAddr — "0100007F:13B0" → (127.0.0.1, 5040). For IPv6 the address
// is 32 hex chars. Kernel writes IP bytes little-endian in 4-byte chunks
// and the port big-endian.
func parseHexAddr(s string, ipv6 bool) (net.IP, int, bool) {
	colon := strings.IndexByte(s, ':')
	if colon < 0 {
		return nil, 0, false
	}
	ipHex := s[:colon]
	portHex := s[colon+1:]
	port, err := strconv.ParseUint(portHex, 16, 16)
	if err != nil {
		return nil, 0, false
	}
	if !ipv6 {
		if len(ipHex) != 8 {
			return nil, 0, false
		}
		b, err := hexToBytesLE(ipHex, 4)
		if err != nil {
			return nil, 0, false
		}
		return net.IPv4(b[0], b[1], b[2], b[3]), int(port), true
	}
	if len(ipHex) != 32 {
		return nil, 0, false
	}
	ip := make(net.IP, 16)
	// Four 32-bit little-endian words.
	for i := 0; i < 4; i++ {
		b, err := hexToBytesLE(ipHex[i*8:(i+1)*8], 4)
		if err != nil {
			return nil, 0, false
		}
		copy(ip[i*4:], b)
	}
	return ip, int(port), true
}

func hexToBytesLE(s string, n int) ([]byte, error) {
	if len(s) != n*2 {
		return nil, fmt.Errorf("bad len")
	}
	out := make([]byte, n)
	for i := 0; i < n; i++ {
		v, err := strconv.ParseUint(s[i*2:i*2+2], 16, 8)
		if err != nil {
			return nil, err
		}
		out[n-1-i] = byte(v) // little-endian: first hex byte = LSB
	}
	return out, nil
}

// displayBind — prettier than raw. 0.0.0.0 / :: become "*".
func displayBind(ip net.IP, ipv6 bool) string {
	if ip.IsUnspecified() {
		return "*"
	}
	// IPv4-mapped IPv6 ("::ffff:127.0.0.1") shows as bare v4.
	if v4 := ip.To4(); v4 != nil {
		return v4.String()
	}
	return ip.String()
}

// ── inode → PID map ───────────────────────────────────────────────────────

func buildInodeMap() map[uint64]int {
	out := map[uint64]int{}
	procEntries, err := os.ReadDir("/proc")
	if err != nil {
		return out
	}
	for _, e := range procEntries {
		if !e.IsDir() {
			continue
		}
		pid, err := strconv.Atoi(e.Name())
		if err != nil {
			continue
		}
		fdDir := fmt.Sprintf("/proc/%d/fd", pid)
		fds, err := os.ReadDir(fdDir)
		if err != nil {
			continue // permission denied on other users' processes
		}
		for _, fd := range fds {
			target, err := os.Readlink(filepath.Join(fdDir, fd.Name()))
			if err != nil {
				continue
			}
			// "socket:[12345]"
			if !strings.HasPrefix(target, "socket:[") || !strings.HasSuffix(target, "]") {
				continue
			}
			inode, err := strconv.ParseUint(target[len("socket:["):len(target)-1], 10, 64)
			if err != nil {
				continue
			}
			// First writer wins — for multi-threaded processes, child
			// threads share fds so we'd see the same inode from several
			// pids. Keeping the lowest pid (thread group leader in /proc
			// ordering) is good enough.
			if _, ok := out[inode]; !ok {
				out[inode] = pid
			}
		}
	}
	return out
}

// ── tiny helpers ──────────────────────────────────────────────────────────

func readFile(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

func readCmdline(pid int) string {
	data, err := os.ReadFile(fmt.Sprintf("/proc/%d/cmdline", pid))
	if err != nil {
		return ""
	}
	s := strings.ReplaceAll(string(data), "\x00", " ")
	return strings.TrimSpace(s)
}

// IsNoise — heuristic filter for processes that aren't "a codebase you're
// running": the SSH daemon, VS Code's Electron workers, and its utility
// sub-processes (Pyright etc.). Kept in the data layer so the UI just
// asks; add more patterns here as new offenders show up.
func (p Port) IsNoise() bool {
	if p.Port == 22 {
		return true
	}
	cmd := p.Cmd
	if strings.Contains(cmd, "/visual-studio-code/") {
		return true
	}
	// Chromium/Electron helpers — Pyright-LSP is the usual culprit.
	if strings.Contains(cmd, "--type=utility") &&
		strings.Contains(cmd, "--utility-sub-type=") {
		return true
	}
	// Jupyter kernels open a pile of ZMQ sockets (shell, iopub, stdin, control,
	// heartbeat) per notebook — useful to the notebook, but not what you'd
	// call "a dev server you're running".
	if strings.Contains(cmd, "ipykernel_launcher") {
		return true
	}
	return false
}

// Project — last three path components of cwd (e.g.
// "repo-automation/sito-template/frontend") so sibling workspaces under a
// monorepo are distinguishable at a glance. Shorter cwds collapse
// gracefully. Falls back to comm, then "?".
func (p Port) Project() string {
	if p.CWD != "" {
		parts := strings.Split(strings.Trim(p.CWD, "/"), "/")
		if n := len(parts); n > 3 {
			parts = parts[n-3:]
		}
		return strings.Join(parts, "/")
	}
	if p.Comm != "" {
		return p.Comm
	}
	return "?"
}
