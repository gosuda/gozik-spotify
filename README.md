# gozik-spotify

Spotify backend source agent for [Gozik](https://github.com/gg582/gozik). It exposes the same `music.v1.MusicProviderService` gRPC interface as `gozik-yt-music`, so the GTK frontend can switch sources transparently.

## Architecture

- **Language**: Go 1.25.6+
- **Protocol**: gRPC (`gozik/api/music/v1/music_provider.proto`)
- **Metadata**: Spotify Web API
- **Audio extraction modes** (selected at runtime via `GOZIK_SPOTIFY_MODE`):
  - `hybrid` (default) — Spotify metadata + `yt-dlp` audio URL resolution
  - `librespot` — placeholder for a local librespot PCM backend

## Configuration

### Required

A Spotify Client ID is required for the Web API PKCE flow. Because gozik-spotify uses the fixed loopback redirect URI `http://127.0.0.1:43827/callback`, **the recommended approach is to register your own Spotify app** and add that URI to it.

The Client ID can be supplied in any of these ways:

1. **Register your own app** (recommended) — create an app at https://developer.spotify.com/dashboard, add `http://127.0.0.1:43827/callback` as a Redirect URI, then paste the app's Client ID into the web UI or Gozik dialog.
2. Set the `GOZIK_SPOTIFY_CLIENT_ID` environment variable.
3. Write it to `~/.config/gozik/spotify_settings.json` as `{ "client_id": "..." }`.
4. **Auto-import from browser** — in the web UI, choose a browser and click **Import & Authenticate**. This reads the Client ID stored by open.spotify.com, but that ID belongs to Spotify's official web player, so most users cannot add the required Redirect URI to it and will see a `redirect_uri` error. Use your own app if this happens.

### Optional

- `GOZIK_SPOTIFY_MODE` — `hybrid` or `librespot` (default: `hybrid`).
- `GOZIK_SPOTIFY_HOST` / `GOZIK_SPOTIFY_PORT` — gRPC bind address (default: `127.0.0.1:50054`, chosen to avoid colliding with `gozik-yt-music` on port 50051).
- `GOZIK_SPOTIFY_WEBUI_PORT` — standalone web UI HTTP port (default: `50055`, set to `0` to disable).
- `GOZIK_SPOTIFY_REGISTER_DESKTOP` — desktop entry behaviour: `auto`, `always`, `never` (default: `auto`).
- `GOZIK_SPOTIFY_NO_POPUP` — set to `1`/`true` to disable the startup GUI popup.
- `GOZIK_SPOTIFY_YTDLP` — path to `yt-dlp` binary (default: system `PATH`, then the `yt-dlp` bundled next to the server binary).
- `XDG_CONFIG_HOME` — config directory (default: `~/.config/gozik/`).

## Build

```bash
cd gozik-spotify
go build ./cmd/gozik-spotify
```

## Install with systemd

### System-wide (requires root)

```bash
make
sudo make install
sudo systemctl daemon-reload
sudo systemctl enable --now gozik-spotify
sudo systemctl status gozik-spotify
```

### User-level (no sudo)

```bash
make
make install-user
systemctl --user daemon-reload
systemctl --user enable --now gozik-spotify
systemctl --user status gozik-spotify
```

Default install paths:
- System: `/usr/local/lib/gozik-spotify/`, `/usr/local/bin/gozik-spotify`, `/etc/systemd/system/gozik-spotify.service`
- User: `~/.local/lib/gozik-spotify/`, `~/.local/bin/gozik-spotify`, `~/.config/systemd/user/gozik-spotify.service`

### Uninstall

```bash
sudo make uninstall        # system
make uninstall-user        # user
```

## Run manually

```bash
export GOZIK_SPOTIFY_CLIENT_ID="your-spotify-client-id"
./gozik-spotify
```

On first use, call `InitiateAuth` from Gozik settings. The server opens the browser to Spotify, captures the PKCE callback on `127.0.0.1:43827`, exchanges the code for tokens, and persists them to `~/.config/gozik/spotify_tokens.json`.

## Standalone web UI

Like `gozik-yt-music`, `gozik-spotify` now serves a built-in web dashboard on a separate HTTP port (default `50055`) so you can manage the plugin with any browser, even without the Gozik desktop app:

```bash
# Open the dashboard
xdg-open http://127.0.0.1:50055
```

The dashboard shows the current auth status, capabilities, and a login page. You can either auto-import a Client ID from a local browser (Chrome, Chromium, Firefox, Edge, Brave, Opera, Vivaldi, Whale) or paste your own Spotify Client ID to complete the PKCE flow.

> **Note:** Browsers installed as Snap or Flatpak packages (common for Firefox on Ubuntu) may block connections to `127.0.0.1:43827` after Spotify redirects back from authentication. If you see "Unable to connect", use a browser installed from a regular package (`.deb`, `.rpm`, etc.) or the system default browser.

Because it speaks plain HTTP, it also answers `curl`:

```bash
curl http://127.0.0.1:50055/api/status
```

On startup, the server tries to show a small GUI popup (via `zenity`, `notify-send`, or `xmessage` on Linux) and registers an app-menu shortcut for the web UI on supported desktops. Disable these with `--no-startup-popup` and `--register-desktop-entry never` if desired.

## Hybrid mode

Search and playlist metadata come from the Spotify Web API. When a track is played, the server builds a deterministic query (`"$TITLE $ARTIST audio"`) and resolves the audio via `yt-dlp`:

```
yt-dlp --no-playlist --extract-audio --audio-format flac -f bestaudio -g "ytsearch1:$query"
```

The `Makefile` downloads `yt-dlp` into `build/` and the install targets ship it alongside the server binary, so no manual `yt-dlp` installation is required. If you want to use your own copy, set `GOZIK_SPOTIFY_YTDLP` or place `yt-dlp` on `PATH`.

## gRPC service

Implements `music.v1.MusicProviderService`:

- `GetProviderMetadata`
- `InitiateAuth` / `CompleteAuth` / `RefreshAuth`
- `Search`
- `GetTrackDetails`
- `ResolveStream`
- `StreamAudio`
- `GetUserLibrary`
- `GetUserPlaylists`
- `GetPlaylistDetails`

## Project layout

```
cmd/gozik-spotify/   # daemon entrypoint
internal/
  auth/              # PKCE OAuth loopback flow
  config/            # token/settings persistence
  spotify/           # Web API client and models
  provider/          # MusicProviderService implementation
  stream/            # audio resolution backends (hybrid / librespot)
```
