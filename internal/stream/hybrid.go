package stream

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	musicv1 "github.com/gg582/gozik/api/music/v1"
)

// Hybrid resolves Spotify tracks by searching YouTube via yt-dlp.
type Hybrid struct {
	ytDLP string
}

// NewHybrid creates a hybrid resolver, locating yt-dlp on PATH or next to the
// gozik-spotify binary so the application bundle is self-contained.
func NewHybrid() (*Hybrid, error) {
	ytdlp := os.Getenv("GOZIK_SPOTIFY_YTDLP")
	if ytdlp != "" {
		if _, err := exec.LookPath(ytdlp); err != nil {
			return nil, fmt.Errorf("GOZIK_SPOTIFY_YTDLP binary not found (%s): %w", ytdlp, err)
		}
		return &Hybrid{ytDLP: ytdlp}, nil
	}

	// Prefer a system yt-dlp on PATH.
	if p, err := exec.LookPath("yt-dlp"); err == nil {
		return &Hybrid{ytDLP: p}, nil
	}

	// Fall back to a yt-dlp shipped in the same directory as this executable.
	exe, err := os.Executable()
	if err == nil {
		exe, _ = filepath.EvalSymlinks(exe)
		bundle := filepath.Dir(exe)
		candidate := filepath.Join(bundle, "yt-dlp")
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			if info.Mode()&0o111 != 0 {
				return &Hybrid{ytDLP: candidate}, nil
			}
		}
	}

	return nil, fmt.Errorf("yt-dlp not found on PATH; install it or set GOZIK_SPOTIFY_YTDLP to the binary path")
}

// Resolve builds a deterministic search query and asks yt-dlp for the best audio URL.
func (h *Hybrid) Resolve(ctx context.Context, track *musicv1.Track) (string, map[string]string, int64, error) {
	if track == nil || track.Id == "" {
		return "", nil, 0, errTrackMissing()
	}

	query := buildSearchQuery(track)
	args := []string{
		"--no-playlist",
		"--extract-audio",
		"--audio-format", "flac",
		"-f", "bestaudio",
		"-g",
		"ytsearch1:" + query,
	}

	cmd := exec.CommandContext(ctx, h.ytDLP, args...)
	out, err := cmd.Output()
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok && len(ee.Stderr) > 0 {
			return "", nil, 0, fmt.Errorf("yt-dlp failed: %s", strings.TrimSpace(string(ee.Stderr)))
		}
		return "", nil, 0, fmt.Errorf("yt-dlp failed: %w", err)
	}

	scanner := bufio.NewScanner(strings.NewReader(string(out)))
	var url string
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line != "" {
			url = line
			break
		}
	}
	if url == "" {
		return "", nil, 0, fmt.Errorf("yt-dlp returned no stream URL")
	}

	// yt-dlp audio URLs are typically valid for several hours; report one hour.
	expiryMs := time.Now().Add(time.Hour).UnixMilli()
	return url, nil, expiryMs, nil
}

func buildSearchQuery(track *musicv1.Track) string {
	parts := []string{track.Title}
	for _, a := range track.Artists {
		if a.Name != "" {
			parts = append(parts, a.Name)
		}
	}
	parts = append(parts, "audio")
	return strings.Join(parts, " ")
}
