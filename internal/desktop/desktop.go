// Package desktop registers a Linux desktop entry so users can open the
// gozik-spotify web UI from the app menu without remembering a port number.
package desktop

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/gg582/gozik-spotify/internal/webui"
)

// Register creates the app-menu shortcut for the web UI if it does not already
// exist (unless force is true). It is best-effort and logs warnings instead of
// returning hard errors.
func Register(port int, force bool) error {
	if os.Getenv("XDG_CURRENT_DESKTOP") == "" && os.Getenv("DESKTOP_SESSION") == "" {
		// Likely a headless environment; skip silently.
		return nil
	}

	path := webui.DesktopFilePath()
	if !force {
		if _, err := os.Stat(path); err == nil {
			return nil
		}
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create applications dir: %w", err)
	}

	contents := fmt.Sprintf(`[Desktop Entry]
Name=gozik Spotify Web UI
Comment=Manage the gozik Spotify plugin
Exec=xdg-open http://127.0.0.1:%d
Type=Application
Terminal=false
Icon=audio-x-generic
Categories=AudioVideo;Audio;
`, port)

	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		return fmt.Errorf("write desktop entry: %w", err)
	}
	return nil
}
