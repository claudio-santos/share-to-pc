# ShareToPC Server

Go webserver that receives URLs from Android and opens them in the PC browser or mpv player.

## Run

```bash
go run .
```

Or build an executable:

```bash
go build -o share-to-pc.exe .
```

The server runs on port **8888** (configurable) and prints its local address on startup.

## Player (mpv + yt-dlp)

When **mpv** is installed on the PC, shared URLs are played in a fullscreen mpv window instead of the browser. The phone app then shows a Remote screen with playback controls.

The share mode can be chosen per share (`mpv` / `browser` / `auto`):
- `mpv` — forces mpv; returns `409` if mpv is not installed.
- `browser` — opens in the **browser player** (a dedicated Chromium window controlled via CDP), falling back to the default browser if no Chromium is found.
- `auto` (default) — mpv if installed, otherwise the browser player.

A URL that mpv/yt-dlp cannot load (DRM, private, non-media page) fails in mpv mode — pick `browser` and share again.

### Browser player (CDP)

Browser mode uses a **dedicated Chromium window** (Brave → Edge → Chrome, in that order) launched with `--remote-debugging-port=9222` and a private profile. The server controls its `<video>` element via the Chrome DevTools Protocol:

- Play/pause, seek, speed and fullscreen work through the same `/remote/cmd` verbs as mpv. Fullscreen puts the `<video>` element in fullscreen (falling back to window fullscreen on pages without a video).
- `GET /api/player` reports `"source":"browser"`, so the app hides mpv-only features (tracks).
- One dedicated tab is shared: the next share reuses it instead of opening a new one.
- `quit` stops playback and **exits the dedicated browser entirely** (it belongs to the remote only). If a pre-existing CDP browser was adopted instead, `quit` only closes the playback tab.
- DRM sites (Netflix, Prime Video) require a **one-time sign-in** in the dedicated profile (`%LOCALAPPDATA%\ShareToPC\browser-profile`). There is no adblock, except Brave's built-in Shields. Extensions and logins are installed once inside the ShareToPC window and persist in that profile.
- The CDP endpoint listens on `127.0.0.1:9222` with no token. Never add a firewall rule for it — only the ShareToPC port (8888) is exposed on the LAN.
- Firefox is not supported by the browser player; such URLs keep the old fire-and-forget behavior (default browser).

### Install

Install [mpv](https://mpv.io/) and [yt-dlp](https://github.com/yt-dlp/yt-dlp). Both must be in PATH.

```bash
winget install mpv
winget install yt-dlp
```

Keep yt-dlp updated (`yt-dlp -U`) — YouTube changes can break it.

### Configuration

- Optional: set `mpvPath` in `config.json` if mpv is not in PATH:
  ```json
  {"port": "8888", "mpvPath": "C:\\tools\\mpv\\mpv.exe"}
  ```
- Optional: set `browserPath` in `config.json` to pin the browser for the browser player (overrides auto-detection):
  ```json
  {"port": "8888", "browserPath": "C:\\Program Files\\Google\\Chrome\\Application\\chrome.exe"}
  ```
- If mpv is not installed, shares open in the browser (v1.0 behavior).
- No new ports or firewall rules — all traffic stays on 8888 (9222 is localhost-only).

### Known limits

- Live streams cannot be seeked (platform limitation).
- DRM-protected and private videos will fail in mpv; use the browser player instead.
- A personal `mpv.conf` can override `--fs=yes`.
- YouTube auto captions require two lines in `portable_config/mpv.conf` (e.g. beside the executable): `ytdl-sub-langs=pt.*,en.*` and `ytdl-raw-options=write-auto-subs=`. They apply only to a fresh mpv launch. Without them, no subtitle tracks appear in `/api/tracks`.

## Windows Firewall

Windows blocks inbound connections by default. Allow the used port (default 8888) on private networks so the phone can reach the server. When Windows shows the firewall prompt on first run, select "Allow". If you missed it, run as Administrator:

```bash
netsh advfirewall firewall add rule name="ShareToPC" dir=in action=allow protocol=TCP localport=8888
```

## Usage

Open `http://localhost:8888` in the PC browser. The page has two sections:

- **Open URL:** paste a URL (e.g. a YouTube link) and submit it.
- **Android Setup:** change the port, pick the network IP and use the QR code to configure the phone app.

### Configure the ShareToPC app

1. Open the page on the PC.
2. Pick the IP of the Wi-Fi network where the PC and the phone are.
3. On the phone, open ShareToPC, tap **Scan QR Code** and point the camera at the QR code on the page. Or type the IP and port by hand.

To change the port, edit the value and save. It applies when the server restarts. The port is stored in `config.json` (created next to the executable on first run).

## API

| Method | Route | Description |
|--------|-------|-------------|
| `POST` | `/share` | Body `{"url":"https://...","mode":"mpv"\|"browser"\|"auto"}`. `mode` is optional (`"auto"`: mpv if installed, else browser player). Returns `{"status":"ok","mode":"mpv"}` or `{"status":"ok","mode":"browser"}`. If `"mpv"` and mpv is not installed → `409`. |
| `POST` | `/remote/cmd` | Body `{"cmd":"toggle"\|"play"\|"pause"\|"seek"\|"seek_to"\|"set_speed"\|"fullscreen","value":<float>}`. Controls playback in either player. `value` is optional for `seek` (default 10), required for `seek_to` and `set_speed`. |
| `POST` | `/remote/cmd` | Body `{"cmd":"set_track","type":"audio"\|"sub"\|"video","value":<id>}`. Selects the audio/subtitle/video track. `value` 0 or negative disables the track (`no`). mpv only; `409` in browser mode. |
| `POST` | `/remote/cmd` | Body `{"cmd":"set_track","type":"edition","value":<id>}`. Selects the video quality edition; restarts playback. `value` must be >= 0. mpv only. Editions are mpv's stock `edition-list` from yt-dlp (identical to "Video" qualities). |
| `POST` | `/remote/cmd` | Body `{"cmd":"quit"}`. Stops playback — closes mpv, or exits the dedicated browser (closes only the playback tab if an external CDP browser was adopted). |
| `GET` | `/api/player` | Returns `{"source":"mpv"\|"browser","available":bool,"running":bool,"playing":bool,"title":string,"pos":float,"duration":float,"speed":float}`. |
| `GET` | `/api/tracks` | Returns `{"video":[...],"audio":[...],"sub":[...],"editions":[...]}` where each item is `{"id":int,"lang":string,"title":string,"selected":bool}`. Reports the mpv track list (`track-list`) and video editions (`edition-list`). |
| `GET` | `/` | Web page (usage + setup). |
| `GET` | `/remote` | Web Remote page — thin playback controls for any device on the LAN, no app needed. |
| `GET` | `/api/config` | Returns `{"port":<num>,"ips":["...",...]}`. |
| `POST` | `/api/config` | Body `{"port":"8888"}`. Saves the port (applies on restart). |
| `POST` | `/api/qrcode` | Body `{"url":"http://..."}`. Returns a PNG of the QR code. |

## Requirements

Go 1.24+ to compile. The generated executable runs without Go installed.
