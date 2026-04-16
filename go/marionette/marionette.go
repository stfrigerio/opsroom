// Package marionette speaks Firefox's built-in automation protocol. A Firefox
// launched with `--marionette` (or `marionette.enabled = true` in about:config)
// listens on 127.0.0.1:2828 and accepts a small set of WebDriver commands we
// use to focus an existing tab or open a new one without duplicating URLs.
//
// Protocol framing is length-prefixed JSON: `<N>:<payload>` where N is ASCII
// decimal byte count. Every payload is a JSON array:
//
//	command  → [0, msgID, "Command:Name", params]
//	response → [1, msgID, errorOrNull, resultOrNull]
//
// We speak just enough of the WebDriver dialect to list tabs, switch, read
// URLs, and open a new one.
package marionette

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"time"
)

const (
	defaultAddr    = "127.0.0.1:2828"
	defaultTimeout = 1200 * time.Millisecond
)

type Client struct {
	conn  net.Conn
	r     *bufio.Reader
	msgID int
}

// Dial opens a Marionette connection, consumes the handshake, and starts a
// WebDriver session. Caller must Close when done.
func Dial() (*Client, error) {
	conn, err := net.DialTimeout("tcp", defaultAddr, defaultTimeout)
	if err != nil {
		return nil, err
	}
	c := &Client{conn: conn, r: bufio.NewReader(conn)}
	c.touchDeadline()

	// Handshake: server sends {"applicationType":"gecko","marionetteProtocol":3}.
	if _, err := c.readFrame(); err != nil {
		conn.Close()
		return nil, fmt.Errorf("marionette handshake: %w", err)
	}
	if _, err := c.command("WebDriver:NewSession", map[string]any{
		"capabilities": map[string]any{"alwaysMatch": map[string]any{}},
	}); err != nil {
		conn.Close()
		return nil, fmt.Errorf("marionette new session: %w", err)
	}
	return c, nil
}

func (c *Client) Close() error { return c.conn.Close() }

// FocusURL scans open tabs; if any satisfies `match`, switches to it and
// returns true. Otherwise opens `fallback` in a fresh tab and returns false.
func (c *Client) FocusURL(match func(url string) bool, fallback string) (bool, error) {
	handles, err := c.handles()
	if err != nil {
		return false, err
	}
	for _, h := range handles {
		if err := c.switchTo(h); err != nil {
			continue
		}
		u, err := c.currentURL()
		if err != nil {
			continue
		}
		if match(u) {
			return true, nil
		}
	}
	return false, c.openTab(fallback)
}

// ── low-level helpers ─────────────────────────────────────────────────────

func (c *Client) touchDeadline() {
	_ = c.conn.SetDeadline(time.Now().Add(defaultTimeout))
}

func (c *Client) command(name string, params any) (json.RawMessage, error) {
	c.msgID++
	id := c.msgID
	body, err := json.Marshal([]any{0, id, name, params})
	if err != nil {
		return nil, err
	}
	c.touchDeadline()
	if _, err := fmt.Fprintf(c.conn, "%d:", len(body)); err != nil {
		return nil, err
	}
	if _, err := c.conn.Write(body); err != nil {
		return nil, err
	}
	// Loop so out-of-order / async frames (rare, but possible) don't trip us.
	for {
		frame, err := c.readFrame()
		if err != nil {
			return nil, err
		}
		var arr []json.RawMessage
		if err := json.Unmarshal(frame, &arr); err != nil || len(arr) < 4 {
			continue
		}
		var mt int
		_ = json.Unmarshal(arr[0], &mt)
		if mt != 1 {
			continue
		}
		var respID int
		_ = json.Unmarshal(arr[1], &respID)
		if respID != id {
			continue
		}
		if len(arr[2]) > 0 && string(arr[2]) != "null" {
			return nil, fmt.Errorf("marionette: %s", string(arr[2]))
		}
		return arr[3], nil
	}
}

func (c *Client) readFrame() ([]byte, error) {
	var sz int
	digits := 0
	for {
		b, err := c.r.ReadByte()
		if err != nil {
			return nil, err
		}
		if b == ':' {
			if digits == 0 {
				return nil, fmt.Errorf("empty frame length")
			}
			break
		}
		if b < '0' || b > '9' {
			return nil, fmt.Errorf("bad frame length byte %q", b)
		}
		sz = sz*10 + int(b-'0')
		digits++
		if sz > 64*1024*1024 {
			return nil, fmt.Errorf("frame too large (%d)", sz)
		}
	}
	buf := make([]byte, sz)
	if _, err := io.ReadFull(c.r, buf); err != nil {
		return nil, err
	}
	return buf, nil
}

// unwrap accepts both shapes Marionette returns in the wild: a raw value
// (older / Marionette-native) or a {"value": X} envelope (WebDriver style).
func unwrap[T any](raw json.RawMessage) (T, error) {
	var zero T
	var direct T
	if err := json.Unmarshal(raw, &direct); err == nil {
		return direct, nil
	}
	var wrap struct {
		Value T `json:"value"`
	}
	if err := json.Unmarshal(raw, &wrap); err != nil {
		return zero, err
	}
	return wrap.Value, nil
}

func (c *Client) handles() ([]string, error) {
	res, err := c.command("WebDriver:GetWindowHandles", map[string]any{})
	if err != nil {
		return nil, err
	}
	return unwrap[[]string](res)
}

func (c *Client) switchTo(handle string) error {
	// Modern Firefox keys on "handle"; older versions keyed on "name". Sending
	// both lets the same client span Firefox ESR → nightly.
	_, err := c.command("WebDriver:SwitchToWindow", map[string]any{
		"handle": handle,
		"name":   handle,
		"focus":  true,
	})
	return err
}

func (c *Client) currentURL() (string, error) {
	res, err := c.command("WebDriver:GetCurrentURL", map[string]any{})
	if err != nil {
		return "", err
	}
	return unwrap[string](res)
}

// Title returns the active tab's title. Firefox syncs the OS window title to
// this string, so it's how we identify which Hyprland window owns the tab
// we just focused — essential when Firefox has windows across workspaces.
func (c *Client) Title() (string, error) {
	res, err := c.command("WebDriver:GetTitle", map[string]any{})
	if err != nil {
		return "", err
	}
	return unwrap[string](res)
}

func (c *Client) openTab(url string) error {
	res, err := c.command("WebDriver:NewWindow", map[string]any{
		"type":  "tab",
		"focus": true,
	})
	if err != nil {
		return err
	}
	type newWin struct {
		Handle string `json:"handle"`
	}
	nw, err := unwrap[newWin](res)
	if err == nil && nw.Handle != "" {
		_ = c.switchTo(nw.Handle)
	}
	_, err = c.command("WebDriver:Navigate", map[string]any{"url": url})
	return err
}
