package main

import (
	"encoding/json"
	"fmt"
	"log"
	"math"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"golang.org/x/net/websocket"
)

const (
	cdpPort = 9222
	cdpBase = "http://127.0.0.1:9222"
)

var (
	modeMu     sync.Mutex
	playerMode string // "mpv" | "browser" | "none"

	cdpMu         sync.Mutex
	cdpBrowserWS  string
	cdpTabID      string
	cdpTabWS      string
	cdpFullscreen bool
	cdpSpawned    bool
	cdpPid        int
	cdpLastStatus browserStatus
)

type browserStatus struct {
	Running    bool
	Playing    bool
	Title      string
	Pos        float64
	Duration   float64
	Speed      float64
	Fullscreen bool `json:"fs"`
}

type cdpTabInfo struct {
	ID   string `json:"id"`
	Type string `json:"type"`
	URL  string `json:"url"`
	WS   string `json:"webSocketDebuggerUrl"`
}

func setPlayerMode(m string) {
	modeMu.Lock()
	playerMode = m
	modeMu.Unlock()
}

func getPlayerMode() string {
	modeMu.Lock()
	defer modeMu.Unlock()
	return playerMode
}

// resolveBrowser returns the first Chromium browser found: configured path,
// then Brave, Edge and Chrome in the usual install locations.
func resolveBrowser() string {
	cfg := loadConfig()
	if cfg.BrowserPath != "" {
		return cfg.BrowserPath
	}
	candidates := []string{
		filepath.Join(os.Getenv("LOCALAPPDATA"), "BraveSoftware", "Brave-Browser", "Application", "brave.exe"),
		filepath.Join(os.Getenv("ProgramFiles"), "BraveSoftware", "Brave-Browser", "Application", "brave.exe"),
		filepath.Join(os.Getenv("ProgramFiles(x86)"), "Microsoft", "Edge", "Application", "msedge.exe"),
		filepath.Join(os.Getenv("ProgramFiles"), "Microsoft", "Edge", "Application", "msedge.exe"),
		filepath.Join(os.Getenv("LOCALAPPDATA"), "Google", "Chrome", "Application", "chrome.exe"),
		filepath.Join(os.Getenv("ProgramFiles"), "Google", "Chrome", "Application", "chrome.exe"),
	}
	for _, p := range candidates {
		if p != "" {
			if _, err := os.Stat(p); err == nil {
				return p
			}
		}
	}
	return ""
}

func cdpAlive() bool {
	client := &http.Client{Timeout: 1 * time.Second}
	res, err := client.Get(cdpBase + "/json/version")
	if err != nil {
		return false
	}
	defer res.Body.Close()
	return res.StatusCode == 200
}

// cdpOwnerPid returns the PID currently listening on the CDP port, resolved
// via netstat. Brave can relaunch itself, so the process spawned with
// exec.Command may exit immediately and the real browser process has a
// different PID.
func cdpOwnerPid() int {
	out, err := exec.Command("netstat", "-ano").Output()
	if err != nil {
		return 0
	}
	for _, line := range strings.Split(string(out), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 5 || fields[0] != "TCP" {
			continue
		}
		idx := strings.LastIndex(fields[1], ":")
		if idx < 0 || fields[1][idx+1:] != strconv.Itoa(cdpPort) {
			continue
		}
		if fields[3] != "LISTENING" {
			continue
		}
		if pid, err := strconv.Atoi(fields[4]); err == nil && pid > 0 {
			return pid
		}
	}
	return 0
}

func cdpBrowserWSUrl() string {
	client := &http.Client{Timeout: 1 * time.Second}
	res, err := client.Get(cdpBase + "/json/version")
	if err != nil {
		return ""
	}
	defer res.Body.Close()
	var v struct {
		WS string `json:"webSocketDebuggerUrl"`
	}
	if err := json.NewDecoder(res.Body).Decode(&v); err != nil {
		return ""
	}
	return v.WS
}

// browserProfile returns the dedicated profile directory used by the browser
// player.
func browserProfile() string {
	return filepath.Join(os.Getenv("LOCALAPPDATA"), "ShareToPC", "browser-profile")
}

// ownedBrowser reports whether a browser running on the CDP port was spawned by
// this server (marker file written on spawn, removed on quit).
func ownedBrowser() bool {
	_, err := os.Stat(filepath.Join(browserProfile(), "share-to-pc-owned"))
	return err == nil
}

func setOwnedMarker() error {
	return os.WriteFile(filepath.Join(browserProfile(), "share-to-pc-owned"), []byte("1"), 0644)
}

func clearOwnedMarker() {
	os.Remove(filepath.Join(browserProfile(), "share-to-pc-owned"))
}

// removeSessionFiles deletes the profile's session-restore data so a fresh
// launch does not bring back stale tabs. Session Storage (localStorage, logins)
// is kept untouched.
func removeSessionFiles(profile string) {
	for _, f := range []string{"Last Session", "Current Session", "Last Tabs", "Current Tabs"} {
		os.Remove(filepath.Join(profile, "Default", f))
	}
	os.RemoveAll(filepath.Join(profile, "Default", "Sessions"))
}

// sanitizeCrashState rewrites the profile's Preferences so a browser killed
// with taskkill is not treated as a crash on the next launch, preventing the
// "restore pages" prompt. Only the two exit-state values are changed.
func sanitizeCrashState(profile string) {
	p := filepath.Join(profile, "Default", "Preferences")
	b, err := os.ReadFile(p)
	if err != nil {
		return
	}
	prefs := string(b)
	updated := prefs
	for _, r := range []struct{ old, new string }{
		{`"exit_type":"Crashed"`, `"exit_type":"Normal"`},
		{`"exit_type": "Crashed"`, `"exit_type": "Normal"`},
		{`"exited_cleanly":false`, `"exited_cleanly":true`},
		{`"exited_cleanly": false`, `"exited_cleanly": true`},
	} {
		updated = strings.ReplaceAll(updated, r.old, r.new)
	}
	if updated != prefs {
		if err := os.WriteFile(p, []byte(updated), 0644); err != nil {
			log.Printf("sanitize preferences failed: %v", err)
		}
	}
}

// ensureBrowser adopts the running CDP browser, or spawns a dedicated
// Chromium window with a private profile. It opens the shared playback tab
// directly on targetURL. The returned boolean reports whether the browser was
// spawned by this call.
func ensureBrowser(targetURL string) (bool, error) {
	if cdpAlive() {
		cdpMu.Lock()
		cdpSpawned = false
		cdpPid = 0
		cdpMu.Unlock()
		return false, nil
	}
	exe := resolveBrowser()
	if exe == "" {
		return false, fmt.Errorf("no Chromium browser found")
	}
	profile := browserProfile()
	if err := os.MkdirAll(profile, 0755); err != nil {
		return false, err
	}
	removeSessionFiles(profile)
	sanitizeCrashState(profile)
	args := []string{
		fmt.Sprintf("--remote-debugging-port=%d", cdpPort),
		fmt.Sprintf("--user-data-dir=%s", profile),
		"--autoplay-policy=no-user-gesture-required",
		"--remote-allow-origins=*",
		"--no-first-run",
		"--no-default-browser-check",
		"--no-session-restore",
		"--disable-session-crashed-bubble",
		"--hide-crash-restore-bubble",
		targetURL,
	}
	cmd := exec.Command(exe, args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: syscall.CREATE_NEW_PROCESS_GROUP}
	if err := cmd.Start(); err != nil {
		return false, err
	}
	setOwnedMarker()
	for i := 0; i < 30; i++ {
		time.Sleep(200 * time.Millisecond)
		if cdpAlive() {
			cdpMu.Lock()
			cdpSpawned = true
			if pid := cdpOwnerPid(); pid > 0 {
				cdpPid = pid
			} else if cmd.Process != nil {
				cdpPid = cmd.Process.Pid
			}
			cdpMu.Unlock()
			return true, nil
		}
	}
	return false, fmt.Errorf("browser CDP did not start")
}

func cdpOpenTab(targetURL string) (id, ws string, err error) {
	req, err := http.NewRequest(http.MethodPut,
		fmt.Sprintf("%s/json/new?%s", cdpBase, url.QueryEscape(targetURL)), nil)
	if err != nil {
		return "", "", err
	}
	client := &http.Client{Timeout: 10 * time.Second}
	res, err := client.Do(req)
	if err != nil {
		return "", "", err
	}
	defer res.Body.Close()
	var t struct {
		ID string `json:"id"`
		WS string `json:"webSocketDebuggerUrl"`
	}
	if err := json.NewDecoder(res.Body).Decode(&t); err != nil {
		return "", "", err
	}
	return t.ID, t.WS, nil
}

// listPageTabs returns the browser's page tabs from /json/list.
func listPageTabs() ([]cdpTabInfo, error) {
	client := &http.Client{Timeout: 3 * time.Second}
	res, err := client.Get(cdpBase + "/json/list")
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	var all []cdpTabInfo
	if err := json.NewDecoder(res.Body).Decode(&all); err != nil {
		return nil, err
	}
	var tabs []cdpTabInfo
	for _, t := range all {
		if t.Type == "page" {
			tabs = append(tabs, t)
		}
	}
	return tabs, nil
}

// adoptPlaybackTab leaves a single page tab showing targetURL: picks the tab
// already on targetURL (or the first page tab), closes the others and navigates
// the survivor if needed.
func adoptPlaybackTab(targetURL string) (id, ws string, err error) {
	quoted, err := json.Marshal(targetURL)
	if err != nil {
		return "", "", err
	}
	var tabs []cdpTabInfo
	for i := 0; i < 6; i++ {
		tabs, err = listPageTabs()
		if err != nil {
			return "", "", err
		}
		if len(tabs) > 0 {
			break
		}
		time.Sleep(300 * time.Millisecond)
	}
	if len(tabs) == 0 {
		return cdpOpenTab(targetURL)
	}
	keep := tabs[0]
	for _, t := range tabs {
		if t.URL == targetURL {
			keep = t
			break
		}
	}
	for _, t := range tabs {
		if t.ID != keep.ID {
			if err := cdpCloseTab(t.ID); err != nil {
				log.Printf("close stale tab failed: %v", err)
			}
		}
	}
	if keep.URL != targetURL {
		if _, err := cdpEvaluateRaw(keep.WS, "location.href = "+string(quoted)); err != nil {
			return "", "", err
		}
	}
	return keep.ID, keep.WS, nil
}

// closeBrowserGracefully asks the browser to shut down cleanly via CDP so the
// next launch does not show the session-restore prompt.
func closeBrowserGracefully() error {
	cdpMu.Lock()
	bws := cdpBrowserWS
	cdpMu.Unlock()
	if bws == "" {
		return fmt.Errorf("no browser websocket")
	}
	c, err := cdpDial(bws)
	if err != nil {
		return err
	}
	defer c.close()
	_, err = c.call("Browser.close", nil)
	return err
}

func cdpCloseTab(id string) error {
	req, err := http.NewRequest(http.MethodPut,
		fmt.Sprintf("%s/json/close/%s", cdpBase, id), nil)
	if err != nil {
		return err
	}
	client := &http.Client{Timeout: 5 * time.Second}
	res, err := client.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	return nil
}

type cdpClient struct {
	conn *websocket.Conn
	id   int
}

type cdpMessage struct {
	ID     int             `json:"id"`
	Method string          `json:"method,omitempty"`
	Params map[string]any  `json:"params,omitempty"`
	Result json.RawMessage `json:"result,omitempty"`
	Error  *struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

func cdpDial(wsURL string) (*cdpClient, error) {
	conn, err := websocket.Dial(wsURL, "", "http://127.0.0.1")
	if err != nil {
		return nil, err
	}
	return &cdpClient{conn: conn}, nil
}

func (c *cdpClient) close() {
	if c.conn != nil {
		c.conn.Close()
	}
}

func (c *cdpClient) call(method string, params map[string]any) (json.RawMessage, error) {
	c.id++
	id := c.id
	if err := websocket.JSON.Send(c.conn, cdpMessage{ID: id, Method: method, Params: params}); err != nil {
		return nil, err
	}
	deadline := time.Now().Add(5 * time.Second)
	for {
		if time.Now().After(deadline) {
			return nil, fmt.Errorf("cdp timeout")
		}
		var reply cdpMessage
		if err := websocket.JSON.Receive(c.conn, &reply); err != nil {
			return nil, err
		}
		if reply.ID == id {
			if reply.Error != nil {
				return nil, fmt.Errorf("cdp error: %s", reply.Error.Message)
			}
			return reply.Result, nil
		}
	}
}

// cdpEvaluateRaw runs Runtime.evaluate on the page websocket and returns the
// expression's value.
func cdpEvaluateRaw(wsURL, expr string) (any, error) {
	c, err := cdpDial(wsURL)
	if err != nil {
		return nil, err
	}
	defer c.close()
	res, err := c.call("Runtime.evaluate", map[string]any{
		"expression":    expr,
		"returnByValue": true,
		"awaitPromise":  true,
		"userGesture":   true,
	})
	if err != nil {
		return nil, err
	}
	var r struct {
		Result struct {
			Value any `json:"value"`
		} `json:"result"`
		ExceptionDetails json.RawMessage `json:"exceptionDetails"`
	}
	if err := json.Unmarshal(res, &r); err != nil {
		return nil, err
	}
	if r.ExceptionDetails != nil {
		return nil, fmt.Errorf("evaluate exception")
	}
	return r.Result.Value, nil
}

const cdpStatusExpr = `(() => {
  const v = document.querySelector('video');
  let playing = false, pos = 0, dur = 0, speed = 1;
  let title = document.title || '';
  if (v) {
    playing = !v.paused && !v.ended;
    pos = v.currentTime || 0;
    dur = v.duration || 0;
    speed = v.playbackRate || 1;
    const vt = v.getAttribute('title');
    if (vt) title = vt;
  }
  return JSON.stringify({ playing, pos, duration: dur, title, speed, fs: !!document.fullscreenElement });
})()`

// cdpPollLoop refreshes the cached browser status once per second while the
// browser player is active.
func cdpPollLoop() {
	for {
		time.Sleep(time.Second)
		if getPlayerMode() != "browser" {
			continue
		}
		cdpMu.Lock()
		ws := cdpTabWS
		cdpMu.Unlock()
		if ws == "" {
			continue
		}
		v, err := cdpEvaluateRaw(ws, cdpStatusExpr)
		if err != nil {
			cdpMu.Lock()
			cdpLastStatus.Running = false
			cdpMu.Unlock()
			continue
		}
		s, ok := v.(string)
		if !ok {
			continue
		}
		var st browserStatus
		if json.Unmarshal([]byte(s), &st) == nil {
			cdpMu.Lock()
			st.Running = true
			cdpLastStatus = st
			cdpFullscreen = st.Fullscreen
			cdpMu.Unlock()
		}
	}
}

// cdpPlay starts playback in the dedicated browser window: adopts or spawns
// it, keeps a single playback tab on the target URL and starts tracking it.
func cdpPlay(targetURL string) error {
	quoted, err := json.Marshal(targetURL)
	if err != nil {
		return err
	}
	spawned, err := ensureBrowser(targetURL)
	if err != nil {
		return err
	}
	cdpMu.Lock()
	ws := cdpTabWS
	cdpMu.Unlock()
	if ws != "" {
		if _, err := cdpEvaluateRaw(ws, "location.href = "+string(quoted)); err == nil {
			cdpMu.Lock()
			cdpLastStatus = browserStatus{}
			cdpMu.Unlock()
			return nil
		}
	}
	var tabID, tabWS string
	if spawned || ownedBrowser() {
		tabID, tabWS, err = adoptPlaybackTab(targetURL)
		if err != nil {
			log.Printf("adopt playback tab failed: %v", err)
		}
	}
	if tabID == "" {
		tabID, tabWS, err = cdpOpenTab(targetURL)
		if err != nil {
			return err
		}
	}
	cdpMu.Lock()
	cdpBrowserWS = cdpBrowserWSUrl()
	cdpTabID = tabID
	cdpTabWS = tabWS
	cdpFullscreen = false
	cdpLastStatus = browserStatus{}
	cdpMu.Unlock()
	return nil
}

const cdpFsExpr = `(async () => {
  if (document.fullscreenElement) {
    await document.exitFullscreen();
    return 'exit';
  }
  const el = document.querySelector('.html5-video-player');
  if (el) {
    await el.requestFullscreen();
    return 'enter';
  }
  const v = document.querySelector('video');
  if (!v) return 'novideo';
  await v.requestFullscreen();
  return 'enter';
})()`

// cdpToggleFullscreen puts the browser player in fullscreen: the YouTube
// player container when present, else the <video> element, falling back to
// window fullscreen when the page has no video element.
func cdpToggleFullscreen() error {
	cdpMu.Lock()
	ws := cdpTabWS
	cdpMu.Unlock()
	if ws != "" {
		v, err := cdpEvaluateRaw(ws, cdpFsExpr)
		if err == nil {
			if s, ok := v.(string); ok {
				switch s {
				case "enter":
					cdpMu.Lock()
					cdpFullscreen = true
					cdpMu.Unlock()
					return nil
				case "exit":
					cdpMu.Lock()
					cdpFullscreen = false
					cdpMu.Unlock()
					return nil
				}
			}
		}
	}
	return cdpWindowFullscreen()
}

func cdpWindowFullscreen() error {
	cdpMu.Lock()
	bws := cdpBrowserWS
	tabID := cdpTabID
	next := !cdpFullscreen
	cdpMu.Unlock()
	if bws == "" || tabID == "" {
		return fmt.Errorf("browser player not running")
	}
	c, err := cdpDial(bws)
	if err != nil {
		return err
	}
	defer c.close()
	res, err := c.call("Browser.getWindowForTarget", map[string]any{"targetId": tabID})
	if err != nil {
		return err
	}
	var w struct {
		WindowID int64 `json:"windowId"`
	}
	if err := json.Unmarshal(res, &w); err != nil {
		return err
	}
	state := "normal"
	if next {
		state = "fullscreen"
	}
	if _, err := c.call("Browser.setWindowBounds", map[string]any{
		"windowId": w.WindowID,
		"bounds":   map[string]string{"windowState": state},
	}); err != nil {
		return err
	}
	cdpMu.Lock()
	cdpFullscreen = next
	cdpMu.Unlock()
	return nil
}

func cdpVideoExpr(base string, val float64) string {
	switch base {
	case "toggle":
		return `(() => { const v = document.querySelector('video'); if (v) { if (v.paused) { v.play(); } else { v.pause(); } } })()`
	case "play":
		return `(() => { const v = document.querySelector('video'); if (v) v.play(); })()`
	case "pause":
		return `(() => { const v = document.querySelector('video'); if (v) v.pause(); })()`
	case "seek":
		clamped := math.Max(-300, math.Min(300, val))
		return fmt.Sprintf(`(() => { const v = document.querySelector('video'); if (!v) return; let t = (v.currentTime || 0) + %f; if (v.duration && t > v.duration) t = v.duration; if (t < 0) t = 0; v.currentTime = t; })()`, clamped)
	case "seek_to":
		clamped := val
		if clamped < 0 {
			clamped = 0
		}
		if clamped > 100000 {
			clamped = 100000
		}
		return fmt.Sprintf(`(() => { const v = document.querySelector('video'); if (v) v.currentTime = %f; })()`, clamped)
	case "set_speed":
		return fmt.Sprintf(`(() => { const v = document.querySelector('video'); if (v) v.playbackRate = %f; })()`, val)
	}
	return ""
}

func handleBrowserCmd(w http.ResponseWriter, req remoteCmdRequest) {
	cdpMu.Lock()
	ws := cdpTabWS
	cdpMu.Unlock()

	if req.Cmd == "quit" {
		cdpMu.Lock()
		tabID := cdpTabID
		spawned := cdpSpawned
		pid := cdpPid
		cdpMu.Unlock()
		ours := spawned || ownedBrowser()
		if tabID == "" && !ours {
			writeJSONError(w, http.StatusConflict, "browser player not running")
			return
		}
		if ours {
			if pid == 0 {
				pid = cdpOwnerPid()
			}
			if err := closeBrowserGracefully(); err != nil {
				log.Printf("graceful browser close failed: %v", err)
			}
			for i := 0; i < 10 && cdpAlive(); i++ {
				time.Sleep(200 * time.Millisecond)
			}
			if cdpAlive() && pid > 0 {
				if err := exec.Command("taskkill", "/PID", strconv.Itoa(pid), "/T", "/F").Run(); err != nil {
					log.Printf("taskkill failed: %v", err)
				}
			}
			clearOwnedMarker()
		} else if tabID != "" {
			if err := cdpCloseTab(tabID); err != nil {
				log.Printf("close tab failed: %v", err)
				writeJSONError(w, http.StatusInternalServerError, "failed to close tab")
				return
			}
		}
		cdpMu.Lock()
		cdpTabID = ""
		cdpTabWS = ""
		cdpBrowserWS = ""
		cdpFullscreen = false
		cdpSpawned = false
		cdpPid = 0
		cdpLastStatus = browserStatus{}
		cdpMu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"status":"ok"}`))
		return
	}

	if ws == "" {
		writeJSONError(w, http.StatusConflict, "browser player not running")
		return
	}

	val := req.Value
	if req.Cmd == "seek" && val == 0 {
		val = 10
	}
	if req.Cmd == "set_speed" && val <= 0 {
		writeJSONError(w, http.StatusBadRequest, "invalid speed")
		return
	}
	if req.Cmd == "set_track" {
		writeJSONError(w, http.StatusConflict, "tracks are only available in mpv mode")
		return
	}
	if req.Cmd == "fullscreen" {
		if err := cdpToggleFullscreen(); err != nil {
			log.Printf("fullscreen failed: %v", err)
			writeJSONError(w, http.StatusInternalServerError, "fullscreen failed")
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"status":"ok"}`))
		return
	}

	expr := cdpVideoExpr(req.Cmd, val)
	if expr == "" {
		writeJSONError(w, http.StatusBadRequest, "unknown command: "+req.Cmd)
		return
	}
	if _, err := cdpEvaluateRaw(ws, expr); err != nil {
		log.Printf("browser command failed: %v", err)
		writeJSONError(w, http.StatusInternalServerError, "browser command failed")
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(`{"status":"ok"}`))
}

type playerStatus struct {
	Source    string  `json:"source"`
	Available bool    `json:"available"`
	Running   bool    `json:"running"`
	Playing   bool    `json:"playing"`
	Title     string  `json:"title"`
	Pos       float64 `json:"pos"`
	Duration  float64 `json:"duration"`
	Speed     float64 `json:"speed"`
}

func playerInfo() playerStatus {
	if getPlayerMode() == "browser" {
		cdpMu.Lock()
		st := cdpLastStatus
		cdpMu.Unlock()
		return playerStatus{
			Source:    "browser",
			Available: resolveBrowser() != "",
			Running:   st.Running,
			Playing:   st.Playing,
			Title:     st.Title,
			Pos:       st.Pos,
			Duration:  st.Duration,
			Speed:     st.Speed,
		}
	}
	info := mpvInfo()
	return playerStatus{
		Source:    "mpv",
		Available: info.Available,
		Running:   info.Running,
		Playing:   info.Playing,
		Title:     info.Title,
		Pos:       info.Pos,
		Duration:  info.Duration,
		Speed:     info.Speed,
	}
}
