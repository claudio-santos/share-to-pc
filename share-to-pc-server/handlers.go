package main

import (
	"encoding/json"
	"log"
	"math"
	"net/http"
	"os/exec"
	"strconv"
	"strings"

	"github.com/skip2/go-qrcode"
)

type shareRequest struct {
	URL  string `json:"url"`
	Mode string `json:"mode"`
}

func writeJSONError(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	w.Write([]byte(`{"error":"` + msg + `"}`))
}

func handleShare(w http.ResponseWriter, r *http.Request) {
	var req shareRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	url := strings.TrimSpace(req.URL)
	if !strings.HasPrefix(url, "http://") && !strings.HasPrefix(url, "https://") {
		writeJSONError(w, http.StatusBadRequest, "invalid URL")
		return
	}

	mode := strings.ToLower(req.Mode)

	if mode == "mpv" && !mpvAvailable() {
		writeJSONError(w, http.StatusConflict, "mpv not installed on the PC")
		return
	}

	// "mpv" and ""/"auto" open in mpv when available; "browser" always uses the browser.
	if mode != "browser" && mpvAvailable() {
		if err := ensureMPV(); err != nil {
			log.Printf("mpv ensure failed: %v", err)
			writeJSONError(w, http.StatusInternalServerError, "mpv player failed to start")
			return
		}
		if _, err := sendMPV([]any{"loadfile", url, "replace"}); err != nil {
			log.Printf("mpv loadfile failed: %v", err)
			writeJSONError(w, http.StatusInternalServerError, "mpv failed to load URL")
			return
		}
		log.Printf("Playing in mpv: %s (from %s)", url, r.RemoteAddr)
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"status":"ok","mode":"mpv"}`))
		return
	}

	if err := openInBrowser(url); err != nil {
		log.Printf("Failed to open: %s (from %s): %v", url, r.RemoteAddr, err)
		writeJSONError(w, http.StatusInternalServerError, "failed to open in browser")
		return
	}
	log.Printf("Opened: %s (from %s)", url, r.RemoteAddr)

	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(`{"status":"ok","mode":"browser"}`))
}

func openInBrowser(url string) error {
	return exec.Command("rundll32", "url.dll,FileProtocolHandler", url).Run()
}

type urlFunc struct {
	Port string   `json:"port"`
	IPs  []string `json:"ips"`
}

func handleGetConfig(w http.ResponseWriter, r *http.Request) {
	uf := urlFunc{
		Port: loadConfig().Port,
		IPs:  localIPs(),
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(uf)
}

func handlePostConfig(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Port string `json:"port"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	port, err := strconv.Atoi(req.Port)
	if err != nil || port < 1 || port > 65535 {
		writeJSONError(w, http.StatusBadRequest, "invalid port")
		return
	}
	if err := saveConfig(config{Port: strconv.Itoa(port)}); err != nil {
		writeJSONError(w, http.StatusInternalServerError, "failed to save config")
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(`{"status":"ok","message":"applied when the server restarts"}`))
}

func handleQRCode(w http.ResponseWriter, r *http.Request) {
	var req struct {
		URL string `json:"url"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || !strings.HasPrefix(req.URL, "http://") && !strings.HasPrefix(req.URL, "https://") {
		writeJSONError(w, http.StatusBadRequest, "invalid URL")
		return
	}
	png, err := qrcode.Encode(req.URL, qrcode.Medium, 256)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "failed to generate QR")
		return
	}
	w.Header().Set("Content-Type", "image/png")
	w.Write(png)
}

type remoteCmdRequest struct {
	Cmd   string  `json:"cmd"`
	Value float64 `json:"value"`
	Type  string  `json:"type"`
}

func handleRemoteCmd(w http.ResponseWriter, r *http.Request) {
	var req remoteCmdRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	if req.Cmd == "quit" {
		if !mpvAvailable() {
			writeJSONError(w, http.StatusConflict, "mpv not installed on the PC")
			return
		}
		if err := quitMPV(); err != nil {
			writeJSONError(w, http.StatusConflict, "mpv already stopped")
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"status":"ok"}`))
		return
	}

	if !mpvAvailable() {
		writeJSONError(w, http.StatusConflict, "mpv not installed on the PC")
		return
	}
	if err := ensureMPV(); err != nil {
		writeJSONError(w, http.StatusConflict, "mpv player not running")
		return
	}

	var cmd []any
	switch req.Cmd {
	case "toggle":
		cmd = []any{"cycle", "pause"}
	case "play":
		cmd = []any{"set_property", "pause", false}
	case "pause":
		cmd = []any{"set_property", "pause", true}
	case "seek":
		val := req.Value
		if val == 0 {
			val = 10
		}
		val = math.Max(-300, math.Min(300, val))
		cmd = []any{"seek", val, "relative"}
	case "seek_to":
		val := req.Value
		if val < 0 {
			val = 0
		}
		if val > 100000 {
			val = 100000
		}
		cmd = []any{"set_property", "time-pos", val}
	case "set_track":
		prop := map[string]string{"video": "vid", "audio": "aid", "sub": "sid", "edition": "edition"}[req.Type]
		if prop == "" {
			writeJSONError(w, http.StatusBadRequest, "invalid track type")
			return
		}
		if req.Type == "edition" {
			if req.Value < 0 {
				writeJSONError(w, http.StatusBadRequest, "invalid edition")
				return
			}
			cmd = []any{"set_property", "edition", int(req.Value)}
		} else if req.Value <= 0 {
			cmd = []any{"set_property", prop, "no"}
		} else {
			cmd = []any{"set_property", prop, int(req.Value)}
		}
	default:
		writeJSONError(w, http.StatusBadRequest, "unknown command: "+req.Cmd)
		return
	}

	if _, err := sendMPV(cmd); err != nil {
		log.Printf("mpv command failed: %v", err)
		writeJSONError(w, http.StatusInternalServerError, "mpv command failed")
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(`{"status":"ok"}`))
}

func handleTracks(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(mpvTracks())
}

func handlePlayerStatus(w http.ResponseWriter, r *http.Request) {
	info := mpvInfo()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(info)
}
