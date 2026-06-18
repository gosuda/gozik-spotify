package stream

import (
	"context"
	"fmt"
	"os/exec"

	musicv1 "github.com/gg582/gozik/api/music/v1"
)

// Librespot resolves Spotify tracks using a local librespot binary.
// It is a Mode-A backend for environments that have Spotify Premium and
// want direct decrypted PCM instead of a hybrid fallback.
type Librespot struct {
	binary string
}

// NewLibrespot creates a librespot resolver, locating the binary on PATH.
func NewLibrespot() (*Librespot, error) {
	bin := "librespot"
	if _, err := exec.LookPath(bin); err != nil {
		return nil, fmt.Errorf("librespot binary not found on PATH; install librespot or set GOZIK_SPOTIFY_MODE=hybrid")
	}
	return &Librespot{binary: bin}, nil
}

// Resolve returns a placeholder; full librespot PCM streaming requires a
// local HTTP bridge around the librespot subprocess.
func (l *Librespot) Resolve(ctx context.Context, track *musicv1.Track) (string, map[string]string, int64, error) {
	if track == nil || track.Id == "" {
		return "", nil, 0, errTrackMissing()
	}
	return "", nil, 0, fmt.Errorf("librespot PCM streaming is not yet wired; use GOZIK_SPOTIFY_MODE=hybrid")
}
