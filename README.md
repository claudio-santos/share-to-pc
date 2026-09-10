# ShareToPC

Send links from your Android phone and open them on your PC (Windows), in **mpv** or a **browser**.

Two components:

- [`share-to-pc-app/`](share-to-pc-app/README.md) — Android app (Kotlin + Compose) that captures the share intent, sends the link, and provides a Remote screen to control playback (play/pause, seek, speed, fullscreen, tracks/editions).
- [`share-to-pc-server/`](share-to-pc-server/README.md) — Go webserver that opens shared URLs in mpv (preferred) or in a dedicated Chromium browser window controlled via CDP, serves a settings page and a Web Remote page, and exposes the playback-control API.

No authentication or TLS: the server is meant for a trusted home network. Build, run and usage instructions are in each component's README.