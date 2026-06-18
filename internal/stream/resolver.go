// Package stream resolves Spotify tracks into playable audio streams.
package stream

import (
	"context"
	"fmt"
	"os"
	"strings"

	musicv1 "github.com/gg582/gozik/api/music/v1"
)

// Mode selects the audio extraction backend.
type Mode string

const (
	// ModeHybrid uses Spotify metadata and resolves audio via yt-dlp.
	ModeHybrid Mode = "hybrid"
	// ModeLibrespot uses a local librespot binary for raw PCM.
	ModeLibrespot Mode = "librespot"
)

// CurrentMode returns the active stream mode from the environment.
func CurrentMode() Mode {
	m := strings.ToLower(os.Getenv("GOZIK_SPOTIFY_MODE"))
	switch m {
	case "librespot":
		return ModeLibrespot
	case "hybrid", "":
		return ModeHybrid
	default:
		return ModeHybrid
	}
}

// Resolver turns a Track message into a direct stream URL and headers.
type Resolver interface {
	Resolve(ctx context.Context, track *musicv1.Track) (streamURL string, headers map[string]string, expiryMs int64, err error)
}

// New creates a Resolver for the current mode.
func New() (Resolver, error) {
	switch CurrentMode() {
	case ModeLibrespot:
		return NewLibrespot()
	default:
		return NewHybrid()
	}
}

// errTrackMissing reports a missing or invalid track payload.
func errTrackMissing() error {
	return fmt.Errorf("track is nil or has no id")
}
