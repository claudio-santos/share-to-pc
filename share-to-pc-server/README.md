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

The share mode can be chosen per share (`mpv` / `browser`); there is **no automatic fallback** — a URL that fails to load in mpv (DRM, private, or non-media page) just fails, so pick the other mode and share again.

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
- If mpv is not installed, shares open in the browser (v1.0 behavior).
- No new ports or firewall rules — all traffic stays on 8888.

### Known limits

- Live streams cannot be seeked (platform limitation).
- DRM-protected and private videos will fail.
- A personal `mpv.conf` can override `--fs=yes`.

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
| `POST` | `/share` | Body `{"url":"https://...","mode":"mpv"\|"browser"\|"auto"}`. `mode` is optional (`"auto"`: mpv if installed, else browser). Returns `{"status":"ok","mode":"mpv"}` or `{"status":"ok","mode":"browser"}`. If `"mpv"` and mpv is not installed → `409`. |
| `POST` | `/remote/cmd` | Body `{"cmd":"toggle"\|"play"\|"pause"\|"seek"\|"seek_to","value":<float>}`. Controls mpv playback. `value` is optional for `seek` (default 10), required for `seek_to`. |
| `POST` | `/remote/cmd` | Body `{"cmd":"set_track","type":"audio"\|"sub"\|"video","value":<id>}`. Selects the audio/subtitle/video track. `value` 0 or negative disables the track (`no`). |
| `POST` | `/remote/cmd` | Body `{"cmd":"set_track","type":"edition","value":<id>}`. Selects the video quality edition (from `mpv-youtube-quality`); restarts playback. `value` must be >= 0. |
| `POST` | `/remote/cmd` | Body `{"cmd":"quit"}`. Closes the mpv window/process. |
| `GET` | `/api/player` | Returns `{"available":bool,"running":bool,"playing":bool,"title":string,"pos":float,"duration":float}`. |
| `GET` | `/api/tracks` | Returns `{"video":[...],"audio":[...],"sub":[...],"editions":[...]}` where each item is `{"id":int,"lang":string,"title":string,"selected":bool}`. Reports the mpv track list (`track-list`) and video editions (`edition-list`). |
| `GET` | `/` | Web page (usage + setup). |
| `GET` | `/api/config` | Returns `{"port":<num>,"ips":["...",...]}`. |
| `POST` | `/api/config` | Body `{"port":"8888"}`. Saves the port (applies on restart). |
| `POST` | `/api/qrcode` | Body `{"url":"http://..."}`. Returns a PNG of the QR code. |

## Requirements

Go 1.22+ to compile. The generated executable runs without Go installed.
