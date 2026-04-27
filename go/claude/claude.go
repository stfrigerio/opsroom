// Package claude discovers running Claude sessions and parses their transcripts.
package claude

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	activeWindow   = 20 * time.Minute
	waitingAfter   = 3 * time.Second
	tailBytes      = 512 * 1024
	maxTailLines   = 400
	maxToolResult  = 4000
)

// EventKind — enum of event types we display.
type EventKind string

const (
	KindThinking   EventKind = "thinking"
	KindToolUse    EventKind = "tool_use"
	KindToolResult EventKind = "tool_result"
	KindText       EventKind = "text"
	KindUserPrompt EventKind = "user_prompt"
	KindUnknown    EventKind = "unknown"
)

type Event struct {
	TS       time.Time
	Kind     EventKind
	Summary  string
	ToolName string
}

type Session struct {
	PID             int
	CWD             string
	Transcript      string
	Events          []Event // oldest → newest
	IsWaiting       bool
	IsWorking       bool // transcript is being appended past the last visible event
	LastEventTS     time.Time
	TranscriptMTime time.Time
}

// debugLog — if WALL_DEBUG is set, append a line to $WALL_DEBUG.
func debugLog(format string, args ...any) {
	path := os.Getenv("WALL_DEBUG")
	if path == "" {
		return
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return
	}
	defer f.Close()
	fmt.Fprintf(f, "[%s] ", time.Now().Format("15:04:05.000"))
	fmt.Fprintf(f, format, args...)
	fmt.Fprintln(f)
}

// Discover walks /proc for claude processes, matches each to its transcript,
// parses recent events, and returns one Session per active claude.
func Discover(eventLimit int) ([]Session, error) {
	projectsDir := filepath.Join(os.Getenv("HOME"), ".claude", "projects")
	st, err := os.Stat(projectsDir)
	if err != nil || !st.IsDir() {
		return nil, nil
	}

	pids, err := topLevelClaudePIDs()
	if err != nil {
		return nil, err
	}
	debugLog("scan: pids=%v", pids)

	// Pass 1: pids with --resume <sid> tell us their exact transcript.
	pidTranscript := map[int]string{}
	claimed := map[string]bool{}
	var unresolved []int

	for _, pid := range pids {
		sid := sessionIDFromCmdline(pid)
		cwd, _ := cwdOf(pid)
		debugLog("  pid=%d cwd=%q sid=%q", pid, cwd, sid)
		if sid == "" || cwd == "" {
			unresolved = append(unresolved, pid)
			continue
		}
		cand := filepath.Join(projectsDir, slugFor(cwd), sid+".jsonl")
		if fi, err := os.Stat(cand); err == nil && !fi.IsDir() {
			pidTranscript[pid] = cand
			claimed[cand] = true
			debugLog("    pass1 pid=%d → %s", pid, filepath.Base(cand))
		} else {
			unresolved = append(unresolved, pid)
		}
	}

	// Pass 2: match each pid to the jsonl whose first-event timestamp is
	// closest to the pid's process start time. Robust when multiple claudes
	// share a project dir and don't expose --resume in cmdline — each pid
	// was created at a distinct moment, and its transcript was opened at
	// that same moment. No open-fd required.
	assignments := matchByStartTime(unresolved, projectsDir, claimed)
	for pid, tr := range assignments {
		pidTranscript[pid] = tr
		claimed[tr] = true
		debugLog("    pass2 pid=%d → %s (starttime)", pid, filepath.Base(tr))
	}

	// Pass 3 fallback: project-dir newest unclaimed. Deterministic by pid.
	sort.Ints(unresolved)
	for _, pid := range unresolved {
		if _, ok := pidTranscript[pid]; ok {
			continue
		}
		cwd, _ := cwdOf(pid)
		if cwd == "" {
			continue
		}
		dir := filepath.Join(projectsDir, slugFor(cwd))
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		type candidate struct {
			path  string
			mtime time.Time
		}
		var candidates []candidate
		for _, e := range entries {
			if e.IsDir() || !strings.HasSuffix(e.Name(), ".jsonl") {
				continue
			}
			p := filepath.Join(dir, e.Name())
			if claimed[p] {
				continue
			}
			fi, err := e.Info()
			if err != nil {
				continue
			}
			candidates = append(candidates, candidate{p, fi.ModTime()})
		}
		sort.Slice(candidates, func(i, j int) bool {
			return candidates[i].mtime.After(candidates[j].mtime)
		})
		if len(candidates) == 0 {
			continue
		}
		pidTranscript[pid] = candidates[0].path
		claimed[candidates[0].path] = true
		debugLog("    pass3 pid=%d → %s (dir-newest)", pid, filepath.Base(candidates[0].path))
	}

	// Pass 4 (refresh): if a freshly-written unclaimed jsonl has appeared in
	// the pid's project dir — and its first event falls after the pid's
	// start time — prefer it. Catches `/clear` and session-switch cases
	// where the pid keeps running but starts writing to a new transcript;
	// without this, the pane stays frozen at the old conversation.
	bt := bootTime()
	for pid, currentPath := range pidTranscript {
		cwd, _ := cwdOf(pid)
		if cwd == "" {
			continue
		}
		start := pidStartTime(pid, bt)
		if start.IsZero() {
			continue
		}
		currentFi, err := os.Stat(currentPath)
		if err != nil {
			continue
		}
		currentMtime := currentFi.ModTime()

		dir := filepath.Join(projectsDir, slugFor(cwd))
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		var bestPath string
		var bestMtime time.Time
		for _, e := range entries {
			if e.IsDir() || !strings.HasSuffix(e.Name(), ".jsonl") {
				continue
			}
			p := filepath.Join(dir, e.Name())
			if p == currentPath || claimed[p] {
				continue
			}
			fi, err := e.Info()
			if err != nil || !fi.ModTime().After(currentMtime) {
				continue
			}
			ft := jsonlFirstTimestamp(p)
			if ft.IsZero() || ft.Before(start) {
				continue
			}
			if bestPath == "" || fi.ModTime().After(bestMtime) {
				bestPath = p
				bestMtime = fi.ModTime()
			}
		}
		if bestPath != "" {
			delete(claimed, currentPath)
			pidTranscript[pid] = bestPath
			claimed[bestPath] = true
			debugLog("    pass4 pid=%d switched → %s (fresher than %s)",
				pid, filepath.Base(bestPath), filepath.Base(currentPath))
		}
	}

	now := time.Now()
	var sessions []Session
	for pid, tr := range pidTranscript {
		fi, err := os.Stat(tr)
		if err != nil {
			continue
		}
		if now.Sub(fi.ModTime()) > activeWindow*10 {
			continue
		}
		cwd, _ := cwdOf(pid)
		lines := tailLines(tr, tailBytes, maxTailLines)
		events := parseEvents(lines, eventLimit)
		waiting := isWaiting(events, now)
		mtime := fi.ModTime()
		var last time.Time
		if len(events) > 0 {
			last = events[len(events)-1].TS
		}
		working := isWorking(events, now, mtime, last)
		sessions = append(sessions, Session{
			PID:             pid,
			CWD:             cwd,
			Transcript:      tr,
			Events:          events,
			IsWaiting:       waiting,
			IsWorking:       working,
			LastEventTS:     last,
			TranscriptMTime: mtime,
		})
	}

	// Emit a bare Session for any claude pid that has no transcript yet
	// (fresh process, user hasn't typed anything). Shows up as an empty
	// card instead of being invisible.
	for _, pid := range pids {
		if _, ok := pidTranscript[pid]; ok {
			continue
		}
		cwd, _ := cwdOf(pid)
		sessions = append(sessions, Session{
			PID: pid,
			CWD: cwd,
		})
	}
	return sessions, nil
}

// ProjectName — last path component of cwd, or the cwd itself if empty.
func ProjectName(cwd string) string {
	if cwd == "" {
		return cwd
	}
	return filepath.Base(cwd)
}

// ── internals ──────────────────────────────────────────────────────────────

func topLevelClaudePIDs() ([]int, error) {
	out, err := exec.Command("pgrep", "-x", "claude").Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 1 {
			return nil, nil // no matches
		}
		return nil, err
	}
	var all []int
	for _, tok := range strings.Fields(string(out)) {
		if n, err := strconv.Atoi(tok); err == nil {
			all = append(all, n)
		}
	}
	var result []int
	for _, pid := range all {
		ppid := ppidOf(pid)
		if ppid > 0 && commOf(ppid) == "claude" {
			continue
		}
		result = append(result, pid)
	}
	return result, nil
}

func ppidOf(pid int) int {
	f, err := os.Open(fmt.Sprintf("/proc/%d/status", pid))
	if err != nil {
		return 0
	}
	defer f.Close()
	data, _ := io.ReadAll(f)
	for _, line := range strings.Split(string(data), "\n") {
		if strings.HasPrefix(line, "PPid:") {
			n, _ := strconv.Atoi(strings.TrimSpace(strings.TrimPrefix(line, "PPid:")))
			return n
		}
	}
	return 0
}

func commOf(pid int) string {
	data, err := os.ReadFile(fmt.Sprintf("/proc/%d/comm", pid))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

func cwdOf(pid int) (string, error) {
	return os.Readlink(fmt.Sprintf("/proc/%d/cwd", pid))
}

// matchByStartTime — pair pids to jsonl transcripts by temporal proximity.
// For each pid: get its process start time. For each candidate jsonl in the
// pid's project dir (that isn't already claimed): get the first-event
// timestamp. Hungarian-style greedy: for every (pid, jsonl) pair sorted by
// |pid_start - jsonl_first|, assign if both are unclaimed. Only pairs
// within maxSkew accept — a jsonl much older/newer than the pid wouldn't
// be its session.
func matchByStartTime(pids []int, projectsDir string, claimed map[string]bool) map[int]string {
	// Claude processes can sit idle for many minutes after startup before the
	// user sends the first event, so the skew between pid-start and the
	// transcript's first event can be large. Generous bound; greedy pairing
	// by minimum skew still finds the right pids first.
	const maxSkew = 12 * time.Hour
	bt := bootTime()
	if bt.IsZero() {
		debugLog("    pass2: bootTime zero, skipping")
		return nil
	}
	debugLog("    pass2: bootTime=%s pids=%v", bt.Format(time.RFC3339), pids)

	type pidInfo struct {
		pid   int
		start time.Time
		dir   string
	}
	var pis []pidInfo
	for _, pid := range pids {
		st := pidStartTime(pid, bt)
		if st.IsZero() {
			debugLog("    pass2 pid=%d: starttime zero", pid)
			continue
		}
		cwd, _ := cwdOf(pid)
		if cwd == "" {
			debugLog("    pass2 pid=%d: empty cwd", pid)
			continue
		}
		dir := filepath.Join(projectsDir, slugFor(cwd))
		debugLog("    pass2 pid=%d start=%s dir=%s", pid, st.Format("15:04:05"), dir)
		pis = append(pis, pidInfo{pid: pid, start: st, dir: dir})
	}

	type jsInfo struct {
		path  string
		first time.Time
	}
	jsByDir := map[string][]jsInfo{}
	for _, pi := range pis {
		if _, ok := jsByDir[pi.dir]; ok {
			continue
		}
		entries, err := os.ReadDir(pi.dir)
		if err != nil {
			jsByDir[pi.dir] = nil
			continue
		}
		var js []jsInfo
		for _, e := range entries {
			if e.IsDir() || !strings.HasSuffix(e.Name(), ".jsonl") {
				continue
			}
			p := filepath.Join(pi.dir, e.Name())
			if claimed[p] {
				continue
			}
			ft := jsonlFirstTimestamp(p)
			if ft.IsZero() {
				debugLog("      jsonl %s: zero timestamp", filepath.Base(p))
				continue
			}
			debugLog("      jsonl %s first=%s", filepath.Base(p), ft.Format("15:04:05"))
			js = append(js, jsInfo{path: p, first: ft})
		}
		jsByDir[pi.dir] = js
	}

	type pair struct {
		pid  int
		path string
		skew time.Duration
	}
	var pairs []pair
	for _, pi := range pis {
		for _, js := range jsByDir[pi.dir] {
			d := pi.start.Sub(js.first)
			if d < 0 {
				d = -d
			}
			if d > maxSkew {
				continue
			}
			pairs = append(pairs, pair{pi.pid, js.path, d})
			debugLog("    pass2 cand pid=%d start=%s js=%s first=%s skew=%s",
				pi.pid, pi.start.Format("15:04:05"), filepath.Base(js.path),
				js.first.Format("15:04:05"), d)
		}
	}
	sort.Slice(pairs, func(i, j int) bool { return pairs[i].skew < pairs[j].skew })

	out := map[int]string{}
	usedPath := map[string]bool{}
	for _, p := range pairs {
		if _, ok := out[p.pid]; ok {
			continue
		}
		if usedPath[p.path] {
			continue
		}
		out[p.pid] = p.path
		usedPath[p.path] = true
	}
	return out
}

func bootTime() time.Time {
	data, err := os.ReadFile("/proc/stat")
	if err != nil {
		return time.Time{}
	}
	for _, line := range strings.Split(string(data), "\n") {
		if strings.HasPrefix(line, "btime ") {
			n, err := strconv.ParseInt(strings.TrimSpace(strings.TrimPrefix(line, "btime ")), 10, 64)
			if err != nil {
				return time.Time{}
			}
			return time.Unix(n, 0)
		}
	}
	return time.Time{}
}

// pidStartTime — field 22 of /proc/<pid>/stat, converted to absolute time.
// Uses CLK_TCK=100 (Linux default; overridable per-arch but universally 100).
func pidStartTime(pid int, boot time.Time) time.Time {
	data, err := os.ReadFile(fmt.Sprintf("/proc/%d/stat", pid))
	if err != nil {
		return time.Time{}
	}
	// comm can contain spaces/parens; skip past the last ')'.
	s := string(data)
	rp := strings.LastIndex(s, ")")
	if rp < 0 {
		return time.Time{}
	}
	fields := strings.Fields(s[rp+1:])
	// After "(comm)" the next field is field 3 (state); field 22 is index 19.
	if len(fields) < 20 {
		return time.Time{}
	}
	ticks, err := strconv.ParseInt(fields[19], 10, 64)
	if err != nil {
		return time.Time{}
	}
	return boot.Add(time.Duration(ticks) * time.Second / 100)
}

// jsonlFirstTimestamp — scan the first few lines looking for the earliest
// "timestamp" field. Claude writes a few timestamp-less preamble records
// (permission-mode etc.) before the first real event, so a naive
// first-line parse misses them. We use bufio.Scanner with a large buffer
// cap because a single line can balloon to MBs when the user pastes an
// image — a fixed-size byte read would truncate that line and silently
// reject the whole file, which then breaks pid↔transcript matching.
// Returns zero if nothing usable found.
func jsonlFirstTimestamp(path string) time.Time {
	f, err := os.Open(path)
	if err != nil {
		return time.Time{}
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 4096), 64*1024*1024)
	var earliest time.Time
	for scanned := 0; scanner.Scan() && scanned < 20; scanned++ {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var obj struct {
			Timestamp string `json:"timestamp"`
		}
		if err := json.Unmarshal(line, &obj); err != nil {
			continue
		}
		if obj.Timestamp == "" {
			continue
		}
		t, err := time.Parse(time.RFC3339Nano, obj.Timestamp)
		if err != nil {
			continue
		}
		if earliest.IsZero() || t.Before(earliest) {
			earliest = t
		}
	}
	return earliest
}

// transcriptFromOpenFDs — look for an open jsonl under projectsDir held by
// pid OR any of its descendants. Claude spawns a node worker that actually
// holds the transcript open, so scanning only the top-level pid misses it.
// Returns "" if nothing found.
func transcriptFromOpenFDs(pid int, projectsDir string) string {
	children := descendants(pid)
	for _, p := range append([]int{pid}, children...) {
		if tr := jsonlFDFor(p, projectsDir); tr != "" {
			return tr
		}
	}
	return ""
}

// jsonlFDFor — scan /proc/<pid>/fd/* for a jsonl under projectsDir. "" if none.
func jsonlFDFor(pid int, projectsDir string) string {
	fdDir := fmt.Sprintf("/proc/%d/fd", pid)
	entries, err := os.ReadDir(fdDir)
	if err != nil {
		return ""
	}
	for _, e := range entries {
		target, err := os.Readlink(filepath.Join(fdDir, e.Name()))
		if err != nil {
			continue
		}
		if !strings.HasSuffix(target, ".jsonl") {
			continue
		}
		if !strings.HasPrefix(target, projectsDir) {
			continue
		}
		return target
	}
	return ""
}

// descendants — every pid whose ppid chain includes `root`. Done via a
// single /proc walk building pid→ppid, then DFS. Returns [] on /proc error.
func descendants(root int) []int {
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return nil
	}
	childrenOf := map[int][]int{}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		pid, err := strconv.Atoi(e.Name())
		if err != nil {
			continue
		}
		pp := ppidOf(pid)
		if pp > 0 {
			childrenOf[pp] = append(childrenOf[pp], pid)
		}
	}
	var out []int
	var walk func(int)
	walk = func(p int) {
		for _, c := range childrenOf[p] {
			out = append(out, c)
			walk(c)
		}
	}
	walk(root)
	return out
}

func sessionIDFromCmdline(pid int) string {
	data, err := os.ReadFile(fmt.Sprintf("/proc/%d/cmdline", pid))
	if err != nil {
		return ""
	}
	args := strings.Split(string(data), "\x00")
	for i, a := range args {
		if a == "--resume" && i+1 < len(args) {
			return args[i+1]
		}
	}
	return ""
}

// slugFor mirrors Claude's project-dir naming: every rune outside
// [-a-zA-Z0-9] collapses to `-`. Without this, a cwd like
// "/home/user/Github/bambù" would map to "-home-user-Github-bambù"
// instead of the real "-home-user-Github-bamb-", and the session's
// transcript would be invisible to discovery.
func slugFor(cwd string) string {
	var b strings.Builder
	b.Grow(len(cwd))
	for _, r := range cwd {
		switch {
		case r >= 'a' && r <= 'z',
			r >= 'A' && r <= 'Z',
			r >= '0' && r <= '9',
			r == '-':
			b.WriteRune(r)
		default:
			b.WriteByte('-')
		}
	}
	return b.String()
}

// tailLines reads up to `maxBytes` from the end of `path`, splits on newline,
// returns at most `n` non-empty lines.
func tailLines(path string, maxBytes int64, n int) []string {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()
	fi, err := f.Stat()
	if err != nil {
		return nil
	}
	readFrom := fi.Size() - maxBytes
	if readFrom < 0 {
		readFrom = 0
	}
	if _, err := f.Seek(readFrom, io.SeekStart); err != nil {
		return nil
	}
	data, _ := io.ReadAll(f)
	var lines []string
	for _, ln := range strings.Split(string(data), "\n") {
		if strings.TrimSpace(ln) == "" {
			continue
		}
		lines = append(lines, ln)
	}
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return lines
}

func parseEvents(lines []string, limit int) []Event {
	var out []Event
	for _, raw := range lines {
		ev, ok := parseEvent(raw)
		if !ok {
			continue
		}
		out = append(out, ev)
	}
	if limit > 0 && len(out) > limit {
		out = out[len(out)-limit:]
	}
	return out
}

func parseEvent(raw string) (Event, bool) {
	var obj map[string]any
	if err := json.Unmarshal([]byte(raw), &obj); err != nil {
		return Event{}, false
	}
	ts := parseTS(asString(obj["timestamp"]))
	role := asString(obj["type"])

	msg, _ := obj["message"].(map[string]any)
	if msg == nil {
		msg = map[string]any{}
	}
	content := msg["content"]

	// User prompts sometimes arrive as plain string content.
	if role == "user" {
		if s, ok := content.(string); ok && strings.TrimSpace(s) != "" {
			return Event{TS: ts, Kind: KindUserPrompt, Summary: sanitizeSummary(s)}, true
		}
	}

	arr, ok := content.([]any)
	if !ok || len(arr) == 0 {
		return Event{}, false
	}
	item, ok := arr[0].(map[string]any)
	if !ok {
		return Event{}, false
	}

	t := asString(item["type"])
	switch t {
	case "thinking":
		text := sanitizeSummary(asString(item["thinking"]))
		if text == "" {
			// Extended-thinking content is delivered encrypted in `signature`;
			// the plaintext `thinking` field is blank. Drop these — they'd
			// render as empty rows and clutter the card.
			return Event{}, false
		}
		return Event{TS: ts, Kind: KindThinking, Summary: text}, true

	case "tool_use":
		name := asString(item["name"])
		if name == "" {
			name = "?"
		}
		inp, _ := item["input"].(map[string]any)
		desc := ""
		for _, k := range []string{"description", "command", "file_path", "pattern", "prompt"} {
			if v, ok := inp[k]; ok {
				if s, ok := v.(string); ok && s != "" {
					desc = s
					break
				}
			}
		}
		return Event{
			TS:       ts,
			Kind:     KindToolUse,
			Summary:  sanitizeSummary(desc),
			ToolName: name,
		}, true

	case "tool_result":
		// Tool results are just noise in the wall view — the user cares about
		// what claude did (tool_use) and what it said (text), not the raw
		// output flowing back.
		return Event{}, false

	case "text":
		return Event{
			TS:      ts,
			Kind:    KindText,
			Summary: sanitizeSummary(asString(item["text"])),
		}, true
	}

	// Raw user prompt embedded in an array item.
	if role == "user" && item["tool_use_id"] == nil {
		var text string
		if s, ok := item["text"].(string); ok {
			text = s
		} else if s, ok := item["content"].(string); ok {
			text = s
		}
		if strings.TrimSpace(text) != "" {
			return Event{TS: ts, Kind: KindUserPrompt, Summary: sanitizeSummary(text)}, true
		}
	}
	return Event{}, false
}

// sanitizeSummary strips terminal escape sequences and control characters
// from text that will be rendered inside a TUI card. Shell output from `!`
// commands often carries ANSI color codes, cursor-movement sequences, and
// \r-based progress-bar overwrites — any of which corrupts our column
// accounting and blows the grid layout. We keep only printable characters,
// tabs, and newlines; for \r we collapse each line to the segment after the
// last \r (mirroring what a terminal would actually display).
func sanitizeSummary(s string) string {
	s = stripANSIEscapes(s)
	// Per-line \r overwrite: "step 1\rstep 2\rstep 3" → "step 3".
	lines := strings.Split(s, "\n")
	for i, ln := range lines {
		if j := strings.LastIndex(ln, "\r"); j >= 0 {
			lines[i] = ln[j+1:]
		}
	}
	s = strings.Join(lines, "\n")

	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		if r == '\n' || r == '\t' {
			b.WriteRune(r)
			continue
		}
		if r < 0x20 || r == 0x7f {
			continue
		}
		b.WriteRune(r)
	}
	return strings.TrimSpace(b.String())
}

// stripANSIEscapes removes ESC-introduced sequences: CSI (\x1b[…<letter>),
// OSC (\x1b]…BEL or …ESC\\), and 2-byte escapes. Anything else that starts
// with ESC gets its ESC dropped and payload kept — worst case is a stray
// letter slips through, which is harmless.
func stripANSIEscapes(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for i := 0; i < len(s); {
		if s[i] != 0x1b {
			b.WriteByte(s[i])
			i++
			continue
		}
		i++ // skip ESC
		if i >= len(s) {
			break
		}
		switch s[i] {
		case '[':
			i++
			for i < len(s) {
				c := s[i]
				i++
				if c >= 0x40 && c <= 0x7e {
					break
				}
			}
		case ']':
			i++
			for i < len(s) {
				if s[i] == 0x07 {
					i++
					break
				}
				if s[i] == 0x1b && i+1 < len(s) && s[i+1] == '\\' {
					i += 2
					break
				}
				i++
			}
		default:
			i++
		}
	}
	return b.String()
}

// isWorking — claude is probably mid-response if either
//   (a) the transcript has been modified more recently than the last visible
//       event (hidden blocks — thinking, tool_result — are streaming), or
//   (b) the last visible event is a user prompt or tool-use and it happened
//       within the last few minutes (so a reply is in-flight).
// In both cases the file mtime must still be within the active window so we
// don't light up stale sessions.
func isWorking(events []Event, now, mtime, lastEventTS time.Time) bool {
	if len(events) == 0 {
		return false
	}
	if now.Sub(mtime) > activeWindow {
		return false
	}
	// Transcript being appended past the last visible event = hidden blocks
	// flowing. Use 1s of slack to avoid flapping on every tick.
	if mtime.Sub(lastEventTS) > time.Second {
		return true
	}
	last := events[len(events)-1]
	if last.Kind == KindUserPrompt || last.Kind == KindToolUse {
		return now.Sub(last.TS) < 5*time.Minute
	}
	return false
}

func isWaiting(events []Event, now time.Time) bool {
	if len(events) == 0 {
		return false
	}
	last := events[len(events)-1]
	if last.Kind != KindText {
		return false
	}
	return now.Sub(last.TS) >= waitingAfter
}

func asString(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}

func parseTS(s string) time.Time {
	if s == "" {
		return time.Time{}
	}
	// Claude timestamps are RFC3339 / ISO8601 with optional Z.
	if t, err := time.Parse(time.RFC3339Nano, s); err == nil {
		return t
	}
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t
	}
	return time.Time{}
}
