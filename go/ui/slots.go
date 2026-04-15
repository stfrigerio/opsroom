package ui

import (
	"fmt"

	"opsroom/claude"
	"opsroom/hypr"
)

// computeSlotSuffixes — produce a map[pid]→"#N" for any claudes that share a
// (workspace, project) group with at least one other. Solo cards get an
// empty suffix (omitted). Order of assignment follows `order` so indices
// stay stable across scans.
func computeSlotSuffixes(
	order []int,
	sessions []claude.Session,
	windows []hypr.Window,
) map[int]string {
	byPID := make(map[int]*claude.Session, len(sessions))
	for i := range sessions {
		byPID[sessions[i].PID] = &sessions[i]
	}

	groups := map[string][]int{}
	for _, pid := range order {
		sess := byPID[pid]
		if sess == nil {
			continue
		}
		ws := -1
		if w := hypr.WindowForPID(pid, windows); w != nil {
			ws = w.Workspace
		}
		key := fmt.Sprintf("%d|%s", ws, claude.ProjectName(sess.CWD))
		groups[key] = append(groups[key], pid)
	}

	out := make(map[int]string, len(order))
	for _, pids := range groups {
		if len(pids) < 2 {
			continue
		}
		for i, pid := range pids {
			out[pid] = fmt.Sprintf("#%d", i+1)
		}
	}
	return out
}
