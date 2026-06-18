// Package browserid extracts a Spotify Client ID from locally installed browsers.
//
// It mirrors yt-dlp's --cookies-from-browser idea, but instead of session
// cookies it looks for persisted Spotify application credentials in browser
// storage (Local Storage / webappsstore.sqlite). The search is best-effort and
// falls back to other authentication methods when nothing is found.
package browserid

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
)

// Supported browser names.
const (
	Chrome   = "chrome"
	Chromium = "chromium"
	Edge     = "edge"
	Brave    = "brave"
	Opera    = "opera"
	Vivaldi  = "vivaldi"
	Firefox  = "firefox"
	Safari   = "safari"
	Whale    = "whale"
)

// Spotify Client IDs are 32-character hex strings.
var clientIDRe = regexp.MustCompile(`[0-9a-fA-F]{32}`)

// Result holds a candidate Client ID and where it was found.
type Result struct {
	ClientID string
	Source   string // e.g. "chrome:localStorage:open.spotify.com"
}

// Extract tries to find a Spotify Client ID in the given browser's storage.
// It returns an error when no candidate could be extracted.
func Extract(browser string) (Result, error) {
	b := normalizeBrowser(browser)
	switch b {
	case Chrome, Chromium, Edge, Brave, Opera, Vivaldi, Whale:
		return extractChromium(b)
	case Firefox:
		return extractFirefox()
	case Safari:
		return Result{}, fmt.Errorf("safari storage extraction is not supported")
	default:
		return Result{}, fmt.Errorf("unsupported browser: %q", browser)
	}
}

// normalizeBrowser maps common aliases to the canonical name.
func normalizeBrowser(name string) string {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "google", "google-chrome", "google_chrome", "chrome":
		return Chrome
	case "chromium", "chromium-browser":
		return Chromium
	case "msedge", "microsoft-edge", "microsoft_edge", "edge":
		return Edge
	case "brave":
		return Brave
	case "opera", "opera-gx":
		return Opera
	case "vivaldi":
		return Vivaldi
	case "firefox":
		return Firefox
	case "safari":
		return Safari
	case "whale", "naver-whale":
		return Whale
	default:
		return strings.ToLower(strings.TrimSpace(name))
	}
}

// scoreCandidate scores a hex match by context. Higher is better.
func scoreCandidate(match string, context string) int {
	low := strings.ToLower(context)
	score := 0
	for _, kw := range []string{"client_id", "clientid", "client-id", "sp_client_id"} {
		if strings.Contains(low, kw) {
			score += 10
		}
	}
	for _, kw := range []string{"spotify", "developer.spotify", "open.spotify", "accounts.spotify"} {
		if strings.Contains(low, kw) {
			score += 5
		}
	}
	return score
}

// pickBest chooses the highest-scoring candidate. An empty slice returns an error.
func pickBest(candidates []Result) (Result, error) {
	if len(candidates) == 0 {
		return Result{}, errors.New("no Spotify Client ID candidate found in browser storage")
	}
	best := candidates[0]
	bestScore := scoreCandidate(best.ClientID, best.Source)
	for _, c := range candidates[1:] {
		s := scoreCandidate(c.ClientID, c.Source)
		if s > bestScore {
			bestScore = s
			best = c
		}
	}
	return best, nil
}

// normalizeClientID lowercases the ID for consistent use.
func normalizeClientID(id string) string {
	return strings.ToLower(id)
}
