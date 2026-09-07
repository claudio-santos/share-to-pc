package main

import (
	"encoding/json"
	"log"
	"net/http"
	"os/exec"
	"strconv"
	"strings"

	"github.com/skip2/go-qrcode"
)

type shareRequest struct {
	URL string `json:"url"`
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

	if err := openInBrowser(url); err != nil {
		log.Printf("Failed to open: %s (from %s): %v", url, r.RemoteAddr, err)
		writeJSONError(w, http.StatusInternalServerError, "failed to open in browser")
		return
	}
	log.Printf("Opened: %s (from %s)", url, r.RemoteAddr)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{"status":"ok"}`))
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