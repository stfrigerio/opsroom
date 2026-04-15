// Package kitty drives a remote kitty terminal via `kitty @ send-text`.
//
// Requires in kitty.conf:
//     allow_remote_control yes
//     listen_on unix:${XDG_RUNTIME_DIR}/kitty-$KITTY_PID
package kitty

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// SendText pipes `text` (plus Enter, if withEnter) into the kitty window
// hosting the given claude pid. Text and Enter go in two calls with a short
// gap so Claude's TUI doesn't batch them into a paste.
//
// Returns the resolved `--match` string (e.g. "id:3" or "pid:12345") so the
// caller can surface it for debugging.
func SendText(claudePID int, text string, withEnter bool) (string, error) {
	if _, err := exec.LookPath("kitty"); err != nil {
		return "", fmt.Errorf("kitty not on PATH")
	}
	env := envOf(claudePID)
	socket := env["KITTY_LISTEN_ON"]
	if socket == "" {
		return "", fmt.Errorf(
			"no KITTY_LISTEN_ON in env — add `allow_remote_control yes` and " +
				"`listen_on unix:${XDG_RUNTIME_DIR}/kitty-$KITTY_PID` to kitty.conf, " +
				"then restart the kitty window.",
		)
	}
	match := resolveMatch(socket, claudePID, env)

	if err := kittySend(socket, match, text); err != nil {
		return match, err
	}
	if withEnter {
		time.Sleep(80 * time.Millisecond)
		if err := kittySend(socket, match, "\r"); err != nil {
			return match, err
		}
	}
	return match, nil
}

func kittySend(socket, match, payload string) error {
	cmd := exec.Command(
		"kitty", "@", "--to", socket,
		"send-text", "--match", match, payload,
	)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		first := strings.SplitN(strings.TrimSpace(stderr.String()), "\n", 2)[0]
		if first == "" {
			first = err.Error()
		}
		return fmt.Errorf("%s", first)
	}
	return nil
}

// resolveMatch — find the kitty window containing `claudePID` by querying
// `kitty @ ls` and inspecting each window's `foreground_processes`. Returns
// `id:<N>` on success, or falls back to `id:<KITTY_WINDOW_ID>` from env,
// or to `pid:<claudePID>` as a last resort.
func resolveMatch(socket string, claudePID int, env map[string]string) string {
	if id, err := findWindowByPID(socket, claudePID); err == nil {
		return "id:" + id
	}
	if id := env["KITTY_WINDOW_ID"]; id != "" {
		return "id:" + id
	}
	return fmt.Sprintf("pid:%d", claudePID)
}

// findWindowByPID — parse `kitty @ ls` output to find the window whose
// foreground process list contains `targetPID`.
func findWindowByPID(socket string, targetPID int) (string, error) {
	out, err := exec.Command("kitty", "@", "--to", socket, "ls").Output()
	if err != nil {
		return "", err
	}

	// kitty @ ls emits a list of "OSWindow" entries, each with tabs, each
	// with windows, each with foreground_processes.
	var osWindows []struct {
		Tabs []struct {
			Windows []struct {
				ID                  int `json:"id"`
				ForegroundProcesses []struct {
					PID int `json:"pid"`
				} `json:"foreground_processes"`
			} `json:"windows"`
		} `json:"tabs"`
	}
	if err := json.Unmarshal(out, &osWindows); err != nil {
		return "", err
	}
	for _, osw := range osWindows {
		for _, tab := range osw.Tabs {
			for _, win := range tab.Windows {
				for _, p := range win.ForegroundProcesses {
					if p.PID == targetPID {
						return strconv.Itoa(win.ID), nil
					}
				}
			}
		}
	}
	return "", fmt.Errorf("no kitty window contains pid %d", targetPID)
}

// CanInject — true if the given claude pid has KITTY_LISTEN_ON in its env,
// which means we have a remote-control socket to send text through. The
// window class reported by the compositor is unreliable (users rename it),
// so the env is the real signal.
func CanInject(claudePID int) bool {
	return envOf(claudePID)["KITTY_LISTEN_ON"] != ""
}

// envOf — parse /proc/<pid>/environ into a map.
func envOf(pid int) map[string]string {
	data, err := os.ReadFile(fmt.Sprintf("/proc/%d/environ", pid))
	if err != nil {
		return nil
	}
	out := map[string]string{}
	for _, entry := range bytes.Split(data, []byte{0}) {
		k, v, ok := bytes.Cut(entry, []byte{'='})
		if !ok {
			continue
		}
		out[string(k)] = string(v)
	}
	return out
}
