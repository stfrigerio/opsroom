// Package hypr talks to Hyprland — find the window hosting a pid, jump to it.
package hypr

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strconv"
	"strings"
)

type Window struct {
	Address   string
	PID       int
	Workspace int
	Title     string
	Class     string
}

// Clients returns all Hyprland client windows.
func Clients() ([]Window, error) {
	out, err := exec.Command("hyprctl", "clients", "-j").Output()
	if err != nil {
		return nil, err
	}
	var raw []struct {
		Address   string `json:"address"`
		PID       int    `json:"pid"`
		Workspace struct {
			ID int `json:"id"`
		} `json:"workspace"`
		Title string `json:"title"`
		Class string `json:"class"`
	}
	if err := json.Unmarshal(out, &raw); err != nil {
		return nil, err
	}
	windows := make([]Window, 0, len(raw))
	for _, c := range raw {
		windows = append(windows, Window{
			Address:   c.Address,
			PID:       c.PID,
			Workspace: c.Workspace.ID,
			Title:     c.Title,
			Class:     c.Class,
		})
	}
	return windows, nil
}

// WindowForPID walks up the ppid chain until it finds a pid that owns a
// Hyprland window, then returns that window.
func WindowForPID(pid int, windows []Window) *Window {
	byPID := make(map[int]*Window, len(windows))
	for i := range windows {
		byPID[windows[i].PID] = &windows[i]
	}
	cur := pid
	for cur > 1 {
		if w, ok := byPID[cur]; ok {
			return w
		}
		cur = ppidOf(cur)
		if cur <= 1 {
			break
		}
	}
	return nil
}

// Focus dispatches `hyprctl focuswindow address:<addr>`.
func Focus(address string) error {
	return exec.Command("hyprctl", "dispatch", "focuswindow", "address:"+address).Run()
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
