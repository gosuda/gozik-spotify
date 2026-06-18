// Package auth implements Spotify Authorization Code Flow with PKCE.
package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os/exec"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/gg582/gozik-spotify/internal/config"
)

const (
	redirectURI  = "http://127.0.0.1:43827/callback"
	spotifyAuth  = "https://accounts.spotify.com/authorize"
	spotifyToken = "https://accounts.spotify.com/api/token"
)

// Scopes required by the provider.
var scopes = []string{
	"user-modify-playback-state",
	"user-read-playback-state",
	"playlist-read-private",
	"user-library-read",
	"user-read-private",
}

// Flow coordinates a single PKCE authentication attempt.
type Flow struct {
	clientID         string
	verifier         string
	state            string
	result           *config.TokenStore
	err              error
	done             chan struct{}
	server           *http.Server
	mu               sync.Mutex
	finishOnce       sync.Once
	preferredBrowser string
}

// NewFlow creates a PKCE flow for the given Spotify client ID.
func NewFlow(clientID string) (*Flow, error) {
	if clientID == "" {
		return nil, fmt.Errorf("client ID is required; set GOZIK_SPOTIFY_CLIENT_ID or call SaveSettings")
	}
	verifier, err := generateCodeVerifier()
	if err != nil {
		return nil, fmt.Errorf("generate verifier: %w", err)
	}
	state, err := randomString(32)
	if err != nil {
		return nil, fmt.Errorf("generate state: %w", err)
	}
	return &Flow{
		clientID: clientID,
		verifier: verifier,
		state:    state,
		done:     make(chan struct{}),
	}, nil
}

// AuthURL returns the URL to open in the user's browser.
func (f *Flow) AuthURL() string {
	challenge := generateCodeChallenge(f.verifier)
	v := url.Values{}
	v.Set("client_id", f.clientID)
	v.Set("response_type", "code")
	v.Set("redirect_uri", redirectURI)
	v.Set("code_challenge_method", "S256")
	v.Set("code_challenge", challenge)
	v.Set("state", f.state)
	v.Set("scope", strings.Join(scopes, " "))
	return spotifyAuth + "?" + v.Encode()
}

// DeviceCode returns the PKCE code verifier; in the proto it is used as the
// opaque device_code so CompleteAuth can correlate the result.
func (f *Flow) DeviceCode() string { return f.verifier }

// SetPreferredBrowser hints that the OAuth URL should be opened in the given
// browser instead of the system default. Used when the user selected a specific
// browser (e.g. Firefox) for Client ID extraction and expects the auth tab to
// appear there.
func (f *Flow) SetPreferredBrowser(browser string) {
	f.preferredBrowser = browser
}

// StartServer starts the local loopback capture server and opens the browser.
func (f *Flow) StartServer() error {
	mux := http.NewServeMux()
	mux.HandleFunc("/callback", f.handleCallback)
	f.server = &http.Server{
		Addr:              "127.0.0.1:43827",
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
	}
	go func() {
		if err := f.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Printf("auth server error: %v", err)
		}
	}()
	openBrowser(f.AuthURL(), f.preferredBrowser)
	return nil
}

// Result waits for the callback to be processed and returns the tokens.
func (f *Flow) Result(ctx context.Context) (*config.TokenStore, error) {
	select {
	case <-f.done:
		f.mu.Lock()
		defer f.mu.Unlock()
		if f.err != nil {
			return nil, f.err
		}
		return f.result, nil
	case <-ctx.Done():
		_ = f.server.Shutdown(context.Background())
		return nil, ctx.Err()
	}
}

func (f *Flow) handleCallback(w http.ResponseWriter, r *http.Request) {
	code := r.URL.Query().Get("code")
	gotState := r.URL.Query().Get("state")
	errMsg := r.URL.Query().Get("error")

	if errMsg != "" {
		f.finish(nil, fmt.Errorf("spotify error: %s", errMsg))
		http.Error(w, "authentication failed: "+errMsg, http.StatusBadRequest)
		return
	}
	if gotState != f.state {
		f.finish(nil, fmt.Errorf("state mismatch"))
		http.Error(w, "state mismatch", http.StatusBadRequest)
		return
	}
	if code == "" {
		f.finish(nil, fmt.Errorf("missing authorization code"))
		http.Error(w, "missing authorization code", http.StatusBadRequest)
		return
	}

	ts, err := f.exchange(code)
	if err != nil {
		f.finish(nil, err)
		http.Error(w, "token exchange failed: "+err.Error(), http.StatusInternalServerError)
		return
	}

	f.finish(ts, nil)
	msg := "<html><body><h1>Spotify authentication successful</h1><p>You may close this window.</p></body></html>"
	w.Header().Set("Content-Type", "text/html")
	_, _ = io.WriteString(w, msg)
}

func (f *Flow) exchange(code string) (*config.TokenStore, error) {
	data := url.Values{}
	data.Set("grant_type", "authorization_code")
	data.Set("code", code)
	data.Set("redirect_uri", redirectURI)
	data.Set("client_id", f.clientID)
	data.Set("code_verifier", f.verifier)

	req, err := http.NewRequest(http.MethodPost, spotifyToken, strings.NewReader(data.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("token request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read token response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("token endpoint status %d: %s", resp.StatusCode, string(body))
	}

	var raw struct {
		AccessToken  string `json:"access_token"`
		TokenType    string `json:"token_type"`
		ExpiresIn    int    `json:"expires_in"`
		RefreshToken string `json:"refresh_token"`
		Scope        string `json:"scope"`
	}
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("parse token response: %w", err)
	}

	ts := &config.TokenStore{
		AccessToken:  raw.AccessToken,
		RefreshToken: raw.RefreshToken,
		TokenType:    raw.TokenType,
		ExpiresAt:    time.Now().Add(time.Duration(raw.ExpiresIn) * time.Second).Unix(),
	}
	if err := config.SaveTokens(ts); err != nil {
		return nil, fmt.Errorf("save tokens: %w", err)
	}
	return ts, nil
}

func (f *Flow) finish(ts *config.TokenStore, err error) {
	f.finishOnce.Do(func() {
		f.mu.Lock()
		f.result = ts
		f.err = err
		f.mu.Unlock()
		close(f.done)
		if f.server != nil {
			_ = f.server.Shutdown(context.Background())
		}
	})
}

// RefreshAccessToken obtains a new access token from a refresh token.
func RefreshAccessToken(clientID, refreshToken string) (*config.TokenStore, error) {
	data := url.Values{}
	data.Set("grant_type", "refresh_token")
	data.Set("refresh_token", refreshToken)
	data.Set("client_id", clientID)

	req, err := http.NewRequest(http.MethodPost, spotifyToken, strings.NewReader(data.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("refresh request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read refresh response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("refresh endpoint status %d: %s", resp.StatusCode, string(body))
	}

	var raw struct {
		AccessToken string `json:"access_token"`
		TokenType   string `json:"token_type"`
		ExpiresIn   int    `json:"expires_in"`
		Scope       string `json:"scope"`
	}
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("parse refresh response: %w", err)
	}

	ts := &config.TokenStore{
		AccessToken:  raw.AccessToken,
		RefreshToken: refreshToken,
		TokenType:    raw.TokenType,
		ExpiresAt:    time.Now().Add(time.Duration(raw.ExpiresIn) * time.Second).Unix(),
	}
	if err := config.SaveTokens(ts); err != nil {
		return nil, fmt.Errorf("save tokens: %w", err)
	}
	return ts, nil
}

// ValidateClientID probes the Spotify authorize endpoint to see if the given
// Client ID accepts our fixed redirect URI. It returns nil when the ID is usable
// and an error describing the failure otherwise.
func ValidateClientID(clientID string) error {
	if clientID == "" {
		return fmt.Errorf("client ID is empty")
	}
	verifier, err := generateCodeVerifier()
	if err != nil {
		return fmt.Errorf("generate verifier: %w", err)
	}
	challenge := generateCodeChallenge(verifier)
	state, err := randomString(16)
	if err != nil {
		return fmt.Errorf("generate state: %w", err)
	}

	v := url.Values{}
	v.Set("client_id", clientID)
	v.Set("response_type", "code")
	v.Set("redirect_uri", redirectURI)
	v.Set("code_challenge_method", "S256")
	v.Set("code_challenge", challenge)
	v.Set("state", state)
	v.Set("scope", strings.Join(scopes, " "))

	req, err := http.NewRequest(http.MethodGet, spotifyAuth+"?"+v.Encode(), nil)
	if err != nil {
		return fmt.Errorf("build probe request: %w", err)
	}
	req.Header.Set("Accept", "text/html")

	// Do not follow redirects; we only care about the initial response status.
	client := &http.Client{CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("probe request: %w", err)
	}
	defer resp.Body.Close()

	// Accept any redirect that points to a Spotify login/consent page.
	// Spotify may return 302, 303, or 307 depending on headers/cookies.
	if resp.StatusCode == http.StatusFound ||
		resp.StatusCode == http.StatusSeeOther ||
		resp.StatusCode == http.StatusTemporaryRedirect {
		loc := resp.Header.Get("Location")
		low := strings.ToLower(loc)
		if strings.Contains(low, "/login") || strings.Contains(low, "/authorize") || strings.Contains(low, redirectURI) {
			return nil
		}
		// If Spotify redirects to an error URL, surface it.
		if strings.Contains(low, "error=") {
			return fmt.Errorf("client ID %q rejected by Spotify (redirect: %s)", clientID, loc)
		}
		// Unknown redirect destination; be permissive but log it.
		return nil
	}

	body, _ := io.ReadAll(resp.Body)
	msg := string(body)
	if strings.Contains(msg, "Invalid redirect URI") {
		return fmt.Errorf("client ID %q does not allow redirect URI %q", clientID, redirectURI)
	}
	if strings.Contains(msg, "Invalid client") {
		return fmt.Errorf("client ID %q is invalid or disabled", clientID)
	}
	return fmt.Errorf("client ID %q rejected by Spotify (status %d): %s", clientID, resp.StatusCode, msg)
}

func generateCodeVerifier() (string, error) {
	b := make([]byte, 64)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

func generateCodeChallenge(verifier string) string {
	sum := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

func randomString(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

func openBrowser(url string, preferredBrowser string) {
	// If the user selected a specific browser (e.g. Firefox) for Client ID
	// extraction, open the auth URL in that browser so the visible tab matches
	// their expectation. Fall back to the system default if the specific
	// browser cannot be launched.
	if preferredBrowser != "" {
		openErr := tryOpenBrowser(url, preferredBrowser)
		if openErr == nil {
			return
		}
		log.Printf("openBrowser: preferred browser %q failed: %v, falling back", preferredBrowser, openErr)
	}

	var cmd string
	var args []string
	switch runtime.GOOS {
	case "darwin":
		cmd = "open"
		args = []string{url}
	case "windows":
		cmd = "rundll32"
		args = []string{"url.dll,FileProtocolHandler", url}
	default:
		cmd = "xdg-open"
		args = []string{url}
	}
	go func() {
		if err := exec.Command(cmd, args...).Start(); err != nil {
			log.Printf("openBrowser: failed to open %s with %s: %v", url, cmd, err)
		}
	}()
}

// tryOpenBrowser attempts to launch *url* with the browser named by *browser*.
// It looks for common binary names on PATH and returns nil on the first
// successful start.
func tryOpenBrowser(url, browser string) error {
	for _, bin := range browserBinaryCandidates(browser) {
		path, err := exec.LookPath(bin)
		if err != nil {
			continue
		}
		cmd := exec.Command(path, url)
		if err := cmd.Start(); err != nil {
			log.Printf("tryOpenBrowser: %s start failed: %v", path, err)
			continue
		}
		return nil
	}
	return fmt.Errorf("no usable binary found for browser %q", browser)
}

// browserBinaryCandidates returns likely executable names for a canonical
// browser name. The order matters: more specific distribution names first.
func browserBinaryCandidates(browser string) []string {
	switch strings.ToLower(strings.TrimSpace(browser)) {
	case "firefox":
		return []string{"firefox"}
	case "chrome":
		return []string{"google-chrome-stable", "google-chrome", "chrome"}
	case "chromium":
		return []string{"chromium", "chromium-browser"}
	case "edge":
		return []string{"microsoft-edge", "msedge"}
	case "brave":
		return []string{"brave", "brave-browser"}
	case "opera":
		return []string{"opera", "opera-gx"}
	case "vivaldi":
		return []string{"vivaldi", "vivaldi-stable"}
	case "whale":
		return []string{"whale", "naver-whale"}
	default:
		return []string{browser}
	}
}
