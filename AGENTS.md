# AGENTS.md

## Project
ShareToPC: send shares from the phone (Android) to open on the PC (Windows), via a Go webserver.

## Structure
- `share-to-pc-server/` — Go webserver. Receives URLs (`POST /share` and opens in the browser) and serves a settings page (port, IPs, QR code).
- `share-to-pc-app/` — Android app (Kotlin + Jetpack Compose) that captures the share intent and sends it to the PC.

## Spirit
- **Simple.** Minimal code, no frameworks, no unnecessary abstractions.
- **No unit tests.** Verify manually.
- **Verification:** server: `go build` + running server + HTTP requests (`Invoke-WebRequest`/`curl`) to endpoints (`/`, `/api/config`, `/api/qrcode`). App: install APK on device and share from another app; configure by scanning the QR code (`Scan QR Code` button).

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