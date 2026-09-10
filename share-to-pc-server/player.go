package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"os/exec"
	"strings"
	"sync"
	"syscall"
	"time"
	"unsafe"
)

const (
	pipeName        = `\\.\pipe\mpv-ipc`
	pipeReadTimeout = 3 * time.Second
	errNoData       = syscall.Errno(232) // ERROR_NO_DATA
	errPipeBusy     = syscall.Errno(231) // ERROR_PIPE_BUSY
)

var (
	kernel32                = syscall.NewLazyDLL("kernel32.dll")
	setNamedPipeHandleState = kernel32.NewProc("SetNamedPipeHandleState")
	waitNamedPipe           = kernel32.NewProc("WaitNamedPipeW")

	mpvMu   sync.Mutex // guards all mpv state
	mpvConn syscall.Handle
	mpvCmd  *exec.Cmd
)

func mpvExecutable() string {
	cfg := loadConfig()
	if cfg.MpvPath != "" {
		return cfg.MpvPath
	}
	path, err := exec.LookPath("mpv")
	if err != nil {
		return ""
	}
	return path
}

func mpvAvailable() bool {
	return mpvExecutable() != ""
}

func setPipeReadTimeout(h syscall.Handle) {
	mode := uint32(0)     // PIPE_READMODE_BYTE | PIPE_WAIT
	waitMs := uint32(200) // per-read wait before returning ERROR_NO_DATA
	setNamedPipeHandleState.Call(
		uintptr(h),
		uintptr(unsafe.Pointer(&mode)),
		0,
		uintptr(unsafe.Pointer(&waitMs)),
		0,
	)
}

func openPipeRaw() (syscall.Handle, error) {
	namep, err := syscall.UTF16PtrFromString(pipeName)
	if err != nil {
		return 0, err
	}
	return syscall.CreateFile(
		namep,
		syscall.GENERIC_READ|syscall.GENERIC_WRITE,
		0,
		nil,
		syscall.OPEN_EXISTING,
		0,
		0,
	)
}

func openPipeRetry(timeout time.Duration) (syscall.Handle, error) {
	deadline := time.Now().Add(timeout)
	namep, _ := syscall.UTF16PtrFromString(pipeName)
	for {
		r, _, _ := waitNamedPipe.Call(uintptr(unsafe.Pointer(namep)), 500)
		if r != 0 {
			h, err := openPipeRaw()
			if err == nil {
				return h, nil
			}
			if err != errPipeBusy {
				return 0, err
			}
		}
		if time.Now().After(deadline) {
			return 0, fmt.Errorf("pipe not available")
		}
		time.Sleep(50 * time.Millisecond)
	}
}

func pipeServerAlive() bool {
	// WaitNamedPipe returns TRUE if an instance is ready to accept, or reports
	// ERROR_PIPE_BUSY if the (single) instance is already connected to a client.
	namep, _ := syscall.UTF16PtrFromString(pipeName)
	r, _, _ := waitNamedPipe.Call(uintptr(unsafe.Pointer(namep)), 100)
	if r != 0 {
		return true
	}
	return syscall.GetLastError() == errPipeBusy
}

func waitForPipe(timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if pipeServerAlive() {
			return true
		}
		time.Sleep(200 * time.Millisecond)
	}
	return false
}

func disconnectLocked(reason string) {
	if mpvConn != 0 {
		syscall.CloseHandle(mpvConn)
		mpvConn = 0
	}
	log.Printf("mpv connection lost: %s", reason)
}

func spawnMPVLocked() error {
	exe := mpvExecutable()
	if exe == "" {
		return fmt.Errorf("mpv not installed")
	}

	args := []string{
		"--input-ipc-server=mpv-ipc",
		"--idle=yes",
		"--force-window=yes",
		"--fs=yes",
		"--keep-open=yes",
	}
	cmd := exec.Command(exe, args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: syscall.CREATE_NEW_PROCESS_GROUP}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to start mpv: %w", err)
	}

	mpvCmd = cmd
	go func() {
		cmd.Wait()
		mpvMu.Lock()
		if mpvCmd == cmd {
			mpvCmd = nil
		}
		if mpvConn != 0 {
			syscall.CloseHandle(mpvConn)
			mpvConn = 0
		}
		mpvMu.Unlock()
		log.Printf("mpv process exited")
	}()
	return nil
}

// connectLocked opens and retains the pipe connection. Assumes mpvMu held.
func connectLocked() error {
	h, err := openPipeRetry(3 * time.Second)
	if err != nil {
		return err
	}
	setPipeReadTimeout(h)
	mpvConn = h
	log.Printf("connected to mpv")
	return nil
}

// ensureMPVLocked guarantees a live connection: adopts a running mpv, or
// spawns one when no mpv is listening on the pipe. Assumes mpvMu held.
func ensureMPVLocked() error {
	if mpvConn != 0 {
		return nil
	}
	if !pipeServerAlive() {
		if err := spawnMPVLocked(); err != nil {
			return err
		}
		if !waitForPipe(3 * time.Second) {
			return fmt.Errorf("mpv pipe not ready")
		}
	}
	return connectLocked()
}

func ensureMPV() error {
	mpvMu.Lock()
	defer mpvMu.Unlock()
	return ensureMPVLocked()
}

func pipeReadLine(h syscall.Handle) (string, error) {
	deadline := time.Now().Add(pipeReadTimeout)
	var buf bytes.Buffer
	one := make([]byte, 1)
	for {
		if time.Now().After(deadline) {
			return "", fmt.Errorf("read timeout")
		}
		var n uint32
		err := syscall.ReadFile(h, one, &n, nil)
		if err != nil {
			if err == errNoData {
				time.Sleep(20 * time.Millisecond)
				continue
			}
			return "", err
		}
		if n == 0 {
			continue
		}
		if one[0] == '\n' {
			return buf.String(), nil
		}
		buf.WriteByte(one[0])
	}
}

// executeRawLocked writes a command and reads the reply on the existing
// connection. It never reconnects or spawns; returns "not connected" when
// no connection is held. Assumes mpvMu held.
func executeRawLocked(command []any) (any, error) {
	if mpvConn == 0 {
		return nil, fmt.Errorf("not connected")
	}

	payload := map[string]any{"command": command}
	data, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	data = append(data, '\n')

	var written uint32
	if err := syscall.WriteFile(mpvConn, data, &written, nil); err != nil {
		disconnectLocked(err.Error())
		return nil, fmt.Errorf("pipe write: %w", err)
	}

	// mpv interleaves asynchronous event lines (start-file, etc.) with command
	// replies; discard any line that carries an "event" field.
	line, err := pipeReadLine(mpvConn)
	for err == nil {
		var probe map[string]any
		if json.Unmarshal([]byte(line), &probe) == nil {
			if _, isEvent := probe["event"]; !isEvent {
				break
			}
		}
		line, err = pipeReadLine(mpvConn)
	}
	if err != nil {
		disconnectLocked(err.Error())
		return nil, fmt.Errorf("pipe read: %w", err)
	}

	var reply struct {
		Data  any    `json:"data"`
		Error string `json:"error"`
	}
	if err := json.Unmarshal([]byte(line), &reply); err != nil {
		return nil, fmt.Errorf("pipe parse: %w (%s)", err, line)
	}
	if reply.Error != "" && reply.Error != "success" {
		return nil, fmt.Errorf("mpv error: %s", reply.Error)
	}
	return reply.Data, nil
}

func sendMPVLocked(command []any) (any, error) {
	if err := ensureMPVLocked(); err != nil {
		return nil, err
	}
	return executeRawLocked(command)
}

func sendMPV(command []any) (any, error) {
	mpvMu.Lock()
	defer mpvMu.Unlock()
	return sendMPVLocked(command)
}

type mpvInfoResult struct {
	Available bool    `json:"available"`
	Running   bool    `json:"running"`
	Playing   bool    `json:"playing"`
	Title     string  `json:"title"`
	Pos       float64 `json:"pos"`
	Duration  float64 `json:"duration"`
	Speed     float64 `json:"speed"`
}

// quitMPV asks mpv to exit and drops the connection without waiting for a
// reply: mpv closes the pipe right after the quit command, so the missing
// reply must not be treated as an error. Spawning mpv again on the next
// command recreates a fresh connection.
func quitMPV() error {
	mpvMu.Lock()
	defer mpvMu.Unlock()

	if mpvConn == 0 && mpvCmd == nil {
		return fmt.Errorf("not running")
	}

	if mpvConn != 0 {
		payload := map[string]any{"command": []any{"quit"}}
		data, _ := json.Marshal(payload)
		data = append(data, '\n')
		var written uint32
		syscall.WriteFile(mpvConn, data, &written, nil)
		syscall.CloseHandle(mpvConn)
		mpvConn = 0
	}

	if mpvCmd != nil && mpvCmd.Process != nil {
		deadline := time.Now().Add(3 * time.Second)
		for time.Now().Before(deadline) && mpvCmd.ProcessState == nil {
			time.Sleep(50 * time.Millisecond)
		}
		if mpvCmd.ProcessState == nil {
			mpvCmd.Process.Kill()
		}
	}
	return nil
}

type trackItem struct {
	ID       int    `json:"id"`
	Lang     string `json:"lang"`
	Title    string `json:"title"`
	Selected bool   `json:"selected"`
}

type tracksResult struct {
	Video    []trackItem `json:"video"`
	Audio    []trackItem `json:"audio"`
	Sub      []trackItem `json:"sub"`
	Editions []trackItem `json:"editions"`
}

// mpvTracks reads the current track list. Report-only: it never spawns mpv.
func mpvTracks() tracksResult {
	mpvMu.Lock()
	defer mpvMu.Unlock()

	var result = tracksResult{
		Video:    []trackItem{},
		Audio:    []trackItem{},
		Sub:      []trackItem{},
		Editions: []trackItem{},
	}
	if mpvConn == 0 {
		if !pipeServerAlive() {
			return result
		}
		if err := connectLocked(); err != nil {
			return result
		}
	}
	v, err := executeRawLocked([]any{"get_property", "track-list"})
	if err != nil {
		return result
	}
	items, ok := v.([]any)
	if !ok {
		return result
	}
	selectedVideoTitle := ""
	for _, it := range items {
		m, ok := it.(map[string]any)
		if !ok {
			continue
		}
		typ, _ := m["type"].(string)
		var t trackItem
		if idf, ok := m["id"].(float64); ok {
			t.ID = int(idf)
		}
		t.Lang, _ = m["lang"].(string)
		t.Title, _ = m["title"].(string)
		t.Selected, _ = m["selected"].(bool)
		switch typ {
		case "video":
			result.Video = append(result.Video, t)
			if t.Selected {
				selectedVideoTitle = t.Title
			}
		case "audio":
			result.Audio = append(result.Audio, t)
		case "sub":
			result.Sub = append(result.Sub, t)
		}
	}

	if v, err := executeRawLocked([]any{"get_property", "edition-list"}); err == nil {
		if items, ok := v.([]any); ok {
			for _, it := range items {
				m, ok := it.(map[string]any)
				if !ok {
					continue
				}
				var e trackItem
				if idf, ok := m["id"].(float64); ok {
					e.ID = int(idf)
				}
				if title, ok := m["title"].(string); ok {
					e.Title = title
				}
				if sel, ok := m["selected"].(bool); ok {
					e.Selected = sel
				} else if def, ok := m["default"].(bool); ok {
					e.Selected = def
				}
				result.Editions = append(result.Editions, e)
			}
		}
		// mpv does not mark the selected edition here; match the active video
		// track's format id (leading token before " - ").
		if selectedVideoTitle != "" {
			prefix := selectedVideoTitle
			if i := strings.Index(prefix, " - "); i >= 0 {
				prefix = prefix[:i]
			}
			for i := range result.Editions {
				if strings.HasPrefix(result.Editions[i].Title, prefix+" - ") {
					result.Editions[i].Selected = true
					break
				}
			}
		}
	}
	return result
}

func mpvInfo() mpvInfoResult {
	mpvMu.Lock()
	defer mpvMu.Unlock()

	info := mpvInfoResult{Available: mpvAvailable()}

	// Report-only path: never spawns mpv. Adopts a live mpv, probes the
	// connection, and reports what is actually there.
	if mpvConn == 0 {
		if !pipeServerAlive() {
			return info
		}
		if err := connectLocked(); err != nil {
			return info
		}
	}
	if _, err := executeRawLocked([]any{"get_property", "idle-active"}); err != nil {
		return info // connection broken; disconnectLocked cleared it
	}
	info.Running = true

	if v, err := executeRawLocked([]any{"get_property", "pause"}); err == nil {
		info.Playing = v == false
	}
	if v, err := executeRawLocked([]any{"get_property", "media-title"}); err == nil {
		if s, ok := v.(string); ok {
			info.Title = s
		}
	}
	if v, err := executeRawLocked([]any{"get_property", "time-pos"}); err == nil {
		if f, ok := v.(float64); ok {
			info.Pos = f
		}
	}
	if v, err := executeRawLocked([]any{"get_property", "duration"}); err == nil {
		if f, ok := v.(float64); ok {
			info.Duration = f
		}
	}
	if v, err := executeRawLocked([]any{"get_property", "speed"}); err == nil {
		if f, ok := v.(float64); ok {
			info.Speed = f
		}
	}
	return info
}
