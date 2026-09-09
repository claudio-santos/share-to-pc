# AGENTS.md

## Project
ShareToPC: send shares from the phone (Android) to open on the PC (Windows), via a Go webserver.

## Structure
- `share-to-pc-server/` — Go webserver. Receives URLs (`POST /share`), opens in mpv (if installed) or browser, serves a settings page, and exposes playback control (`POST /remote/cmd`, `GET /api/player`, `GET /api/tracks`).
- `share-to-pc-app/` — Android app (Kotlin + Jetpack Compose) that captures the share intent, sends it to the PC, and provides a Remote screen for playback control.

## Spirit
- **Simple.** Minimal code, no frameworks, no unnecessary abstractions.
- **No unit tests.** Verify manually.
- **Verification:** server: `go build` + running server + HTTP requests (`Invoke-WebRequest`/`curl`) to endpoints (`/`, `/api/config`, `/api/qrcode`, `/api/player`, `/api/tracks`) and writes to `/share` (with `mode` `"mpv"`/`"browser"`/`"auto"`) and `/remote/cmd` (`toggle`, `set_track` with `type` `"audio"`/`"sub"`/`"video"`/`"edition"`, `quit`). App: install APK on device and share from another app; configure by scanning the QR code (`Scan QR Code` button); default open mode is Ask (dialog with mpv/Browser + remember, chips at "Open mode"); Remote screen controls mpv playback on the PC: Tracks sheet shows Quality (editions), Audio, Subtitles, Video with scroll and refresh, and `set_track edition` restarts playback.

Note: YouTube auto captions require two lines in mpv's config file (e.g. `portable_config/mpv.conf`): `ytdl-sub-langs=pt.*,en.*` and `ytdl-raw-options=write-auto-subs=` (apply to a fresh mpv launch). Without them, no subtitle tracks appear in `/api/tracks`.

## Build
- Server: `go build -o share-to-pc.exe .` (workdir: `share-to-pc-server/`)
- App: `.\gradlew.bat assembleDebug` (workdir: `share-to-pc-app/`), APK at `app/build/outputs/apk/debug/app-debug.apk`

## Conventions
- **All code, comments and text must be in English.**
- Go: stdlib whenever possible; only add a dependency if it brings real value (e.g. go-qrcode).
- Frontend: HTML/CSS/JS inline in a single file (`static/index.html`), no dependencies.
- Android: Compose UI; use stdlib (`HttpURLConnection`, `SharedPreferences`) and zxing (`com.journeyapps:zxing-android-embedded`) for QR scanning; only add a dependency if it brings real value.
- Kotlin: no unit tests; verify on device by installing the APK.
- Follow the existing style; no unsolicited comments.
- Keep the webserver stateless and predictable (e.g. port applies on restart).