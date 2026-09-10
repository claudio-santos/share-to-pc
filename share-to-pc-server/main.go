package main

import (
	_ "embed"
	"log"
	"net"
	"net/http"
)

//go:embed static/index.html
var indexHTML []byte

//go:embed static/remote.html
var remoteHTML []byte

func main() {
	cfg := loadConfig()

	go cdpPollLoop()

	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write(indexHTML)
	})
	mux.HandleFunc("GET /remote", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write(remoteHTML)
	})
	mux.HandleFunc("POST /share", handleShare)
	mux.HandleFunc("POST /remote/cmd", handleRemoteCmd)
	mux.HandleFunc("GET /api/player", handlePlayerStatus)
	mux.HandleFunc("GET /api/tracks", handleTracks)
	mux.HandleFunc("GET /api/config", handleGetConfig)
	mux.HandleFunc("POST /api/config", handlePostConfig)
	mux.HandleFunc("POST /api/qrcode", handleQRCode)

	log.Printf("ShareToPC running on port %s", cfg.Port)
	log.Printf("Local: http://localhost:%s", cfg.Port)

	log.Fatal(http.ListenAndServe(":"+cfg.Port, mux))
}

func localIPs() []string {
	var ips []string
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return ips
	}
	for _, a := range addrs {
		if ipnet, ok := a.(*net.IPNet); ok && ipnet.IP.To4() != nil {
			ip := ipnet.IP
			if ip.IsLoopback() || ip.IsLinkLocalUnicast() {
				continue
			}
			ips = append(ips, ip.String())
		}
	}
	return ips
}
