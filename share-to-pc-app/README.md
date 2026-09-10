# ShareToPC App

Android app that captures the system share intent and sends links to the ShareToPC server. When the PC plays the URL in mpv, the app switches to a **Remote** screen that controls playback.

## Build

```bash
.\gradlew.bat assembleDebug
```

The APK is at `app/build/outputs/apk/debug/app-debug.apk`.

## Install

Copy the APK to the phone and open it (allow "install from unknown sources" if asked).

## Configure

The app needs the PC's IP and port:

1. Open the ShareToPC server page on the PC (`http://localhost:8888`).
2. On the phone, open ShareToPC and tap **Scan QR Code** to read the QR code from the page (camera permission required on first use). Or type the IP and port by hand and tap **Save**.
3. Tap **Test Connection** to check the PC is reachable.

## Usage

Share a link from any app (e.g. a YouTube link) and pick **ShareToPC** in the share menu. The app extracts the URL and sends it to the PC.

**Open mode** (chips on the setup screen, or the dialog shown per share):

- **Ask** — every share shows a dialog to pick mpv or Browser (with an optional "remember").
- **mpv** — always play in mpv on the PC.
- **Browser** — always open in the PC browser.

Opening in mpv switches the app to the **Remote** screen with:

- Play/pause, seek ±10s, seek slider.
- Fullscreen toggle and a speed dropdown (0.5x–2x), for both mpv and the browser player.
- **Tracks** bottom sheet (mpv only): editions ("Quality"), Audio, Subtitles, Video — with refresh. Selecting an edition restarts playback.

## Requirements

Android 7.0+ (minSdk 24). The phone and the PC must be on the same network. The app is portrait-locked.