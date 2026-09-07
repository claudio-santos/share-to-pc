# ShareToPC Server

Go webserver that receives URLs from Android and opens them in the PC browser.

## Run

```bash
go run .
```

Or build an executable:

```bash
go build -o share-to-pc.exe .
```

The server runs on port **8888** (configurable) and prints its local address on startup.

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
| `POST` | `/share` | Body `{"url":"https://..."}`. Opens the URL in the PC default browser. |
| `GET` | `/` | Web page (usage + setup). |
| `GET` | `/api/config` | Returns `{"port":<num>,"ips":["...",...]}`. |
| `POST` | `/api/config` | Body `{"port":"8888"}`. Saves the port (applies on restart). |
| `POST` | `/api/qrcode` | Body `{"url":"http://..."}`. Returns a PNG of the QR code. |

## Requirements

Go 1.22+ to compile. The generated executable runs without Go installed.