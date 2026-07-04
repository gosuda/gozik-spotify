// Package webui serves a small browser-based dashboard for gozik-spotify.
//
// It mirrors the standalone web console shipped by gozik-yt-music so users can
// authenticate and inspect the Spotify plugin without the main Gozik desktop
// application. The UI is plain HTML/CSS served by net/http and uses only the
// standard library.
package webui

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	musicv1 "github.com/gg582/gozik/api/music/v1"
	"github.com/gg582/gozik-spotify/internal/config"
)

const (
	// DefaultPort is the default HTTP port for the standalone web UI.
	DefaultPort = 50055
)

// MusicProvider is the subset of the gRPC servicer that the web UI needs.
type MusicProvider interface {
	GetProviderMetadata(ctx context.Context, req *musicv1.GetProviderMetadataRequest) (*musicv1.GetProviderMetadataResponse, error)
	InitiateAuth(ctx context.Context, req *musicv1.InitiateAuthRequest) (*musicv1.InitiateAuthResponse, error)
	CompleteAuth(ctx context.Context, req *musicv1.CompleteAuthRequest) (*musicv1.CompleteAuthResponse, error)
}

// Server wraps the HTTP web UI server.
type Server struct {
	provider MusicProvider
	port     int
	srv      *http.Server
}

// New creates a web UI server for the given provider and port.
func New(provider MusicProvider, port int) *Server {
	return &Server{
		provider: provider,
		port:     port,
	}
}

// Start launches the web UI in a background goroutine and returns the server.
func (s *Server) Start() error {
	mux := http.NewServeMux()
	mux.HandleFunc("/", s.handleIndex)
	mux.HandleFunc("/login", s.handleLogin)
	mux.HandleFunc("/api/status", s.handleAPIStatus)
	mux.HandleFunc("/login/initiate", s.handleLoginInitiate)
	mux.HandleFunc("/login/complete", s.handleLoginComplete)
	mux.HandleFunc("/login/extract", s.handleLoginExtract)
	mux.HandleFunc("/logout", s.handleLogout)

	s.srv = &http.Server{
		Addr:              fmt.Sprintf("127.0.0.1:%d", s.port),
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
	}

	go func() {
		log.Printf("Web UI listening on http://127.0.0.1:%d", s.port)
		if err := s.srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Printf("web UI server error: %v", err)
		}
	}()
	return nil
}

// Shutdown gracefully stops the web UI server.
func (s *Server) Shutdown(ctx context.Context) error {
	if s.srv == nil {
		return nil
	}
	return s.srv.Shutdown(ctx)
}

func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		s.sendHTML(w, http.StatusNotFound, notFoundPage())
		return
	}

	meta, err := s.provider.GetProviderMetadata(r.Context(), &musicv1.GetProviderMetadataRequest{})
	if err != nil {
		s.sendHTML(w, http.StatusInternalServerError, errorPage(fmt.Sprintf("status: %v", err)))
		return
	}

	// If not authenticated, redirect to the login page so the app-menu entry
	// immediately shows the auth flow.
	if meta.AuthStatus != musicv1.AuthStatus_AUTH_STATUS_AUTHENTICATED {
		http.Redirect(w, r, "/login", http.StatusFound)
		return
	}

	auth := meta.AuthStatus == musicv1.AuthStatus_AUTH_STATUS_AUTHENTICATED
	statusClass := "no"
	statusText := "Unauthenticated"
	if auth {
		statusClass = "ok"
		statusText = "Authenticated"
	}

	var caps []string
	for _, c := range meta.Capabilities {
		caps = append(caps, capName(c))
	}

	content := fmt.Sprintf(`
<div class="card">
  <h2>Status</h2>
  <p><span class="status %s">%s</span></p>
  <p style="color:var(--muted);font-size:.85rem">
    Provider: <strong>%s</strong> (%s)
  </p>
  %s
  <div class="row" style="margin-top:.75rem">
    <a href="/login"><button>Authenticate</button></a>
    <form method="post" action="/logout" style="display:inline" onsubmit="return confirm('Remove stored Spotify credentials?')">
      <button type="submit" style="background:#333">Logout</button>
    </form>
  </div>
</div>
<div class="card">
  <h2>Capabilities</h2>
  <p style="font-size:.9rem">%s</p>
</div>
`, statusClass, statusText, meta.DisplayName, meta.ProviderId,
		ifElse(auth, "<p>Spotify credentials are stored.</p>", ""),
		strings.Join(caps, "<br>"))

	s.sendHTML(w, http.StatusOK, render(content))
}

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	s.sendHTML(w, http.StatusOK, render(loginPage()))
}

func (s *Server) handleAPIStatus(w http.ResponseWriter, r *http.Request) {
	meta, err := s.provider.GetProviderMetadata(r.Context(), &musicv1.GetProviderMetadataRequest{})
	if err != nil {
		s.sendJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	s.sendJSON(w, http.StatusOK, map[string]any{
		"provider_id":  meta.ProviderId,
		"display_name": meta.DisplayName,
		"auth_status":  meta.AuthStatus.String(),
		"capabilities": meta.Capabilities,
	})
}

func (s *Server) handleLoginInitiate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		s.sendJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "POST required"})
		return
	}

	var body struct {
		ClientID string `json:"client_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		s.sendJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid JSON"})
		return
	}

	params := map[string]string{}
	if strings.TrimSpace(body.ClientID) != "" {
		params["client_id"] = strings.TrimSpace(body.ClientID)
	}

	resp, err := s.provider.InitiateAuth(r.Context(), &musicv1.InitiateAuthRequest{Params: params})
	if err != nil {
		s.sendJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}

	s.sendJSON(w, http.StatusOK, map[string]any{
		"auth_url":    resp.AuthUrl,
		"device_code": resp.DeviceCode,
		"message":     "A browser window has been opened. Please approve Spotify access in that window.",
	})
}

func (s *Server) handleLoginExtract(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		s.sendJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "POST required"})
		return
	}

	var body struct {
		Browser string `json:"browser"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		s.sendJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid JSON"})
		return
	}

	browser := strings.TrimSpace(body.Browser)
	if browser == "" {
		s.sendJSON(w, http.StatusBadRequest, map[string]any{"error": "browser is required"})
		return
	}

	// InitiateAuth will try to extract a Client ID from the selected browser
	// and open the Spotify OAuth dialog. The user must approve access in the
	// opened browser; CompleteAuth is called separately afterwards.
	resp, err := s.provider.InitiateAuth(r.Context(), &musicv1.InitiateAuthRequest{
		Params: map[string]string{"browser": browser},
	})
	if err != nil {
		s.sendJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}

	s.sendJSON(w, http.StatusOK, map[string]any{
		"auth_url": resp.AuthUrl,
		"message":  "Browser window opened. Approve Spotify access, then click Complete.",
	})
}

func (s *Server) handleLoginComplete(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		s.sendJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "POST required"})
		return
	}

	resp, err := s.provider.CompleteAuth(r.Context(), &musicv1.CompleteAuthRequest{})
	if err != nil {
		s.sendJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}

	s.sendJSON(w, http.StatusOK, map[string]any{
		"access_token": resp.AccessToken,
		"expires_at":   resp.ExpiresAt,
	})
}

func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Redirect(w, r, "/", http.StatusFound)
		return
	}

	for _, p := range []string{config.TokenPath(), config.SettingsPath()} {
		if _, err := os.Stat(p); err == nil {
			if err := os.Remove(p); err == nil {
				log.Printf("Removed credential file: %s", p)
			}
		}
	}
	http.Redirect(w, r, "/", http.StatusFound)
}

func (s *Server) sendHTML(w http.ResponseWriter, code int, body string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Content-Length", fmt.Sprintf("%d", len(body)))
	w.WriteHeader(code)
	_, _ = w.Write([]byte(body))
}

func (s *Server) sendJSON(w http.ResponseWriter, code int, data any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(data)
}

func capName(cap musicv1.ProviderCapability) string {
	switch cap {
	case musicv1.ProviderCapability_PROVIDER_CAPABILITY_SEARCH:
		return "Search"
	case musicv1.ProviderCapability_PROVIDER_CAPABILITY_STREAM_TRACK:
		return "Stream Track"
	case musicv1.ProviderCapability_PROVIDER_CAPABILITY_LIBRARY_MANAGEMENT:
		return "Library Management"
	default:
		return cap.String()
	}
}

func notFoundPage() string {
	return render(`<div class="card"><h2>404</h2><p>Page not found.</p><a href="/">← Back to dashboard</a></div>`)
}

func errorPage(msg string) string {
	return render(fmt.Sprintf(`<div class="card"><h2>Error</h2><p>%s</p><a href="/">← Back to dashboard</a></div>`, msg))
}

func loginPage() string {
	return `
<div class="card">
  <h2>Recommended: Register your own Spotify app</h2>
  <p style="font-size:.9rem;color:var(--muted)">
    gozik-spotify needs a Spotify Client ID that allows the loopback redirect URI
    <code>http://127.0.0.1:43827/callback</code>. The easiest way is to create your
    own app in the
    <a href="https://developer.spotify.com/dashboard" target="_blank">Spotify Developer Dashboard</a>.
  </p>
  <ol style="font-size:.9rem;color:var(--muted);padding-left:1.25rem;line-height:1.6">
    <li>Click <strong>Create app</strong> in the Dashboard.</li>
    <li>Paste this exact Redirect URI: <code>http://127.0.0.1:43827/callback</code></li>
    <li>Save the app and copy its <strong>Client ID</strong>.</li>
    <li>Paste the Client ID below and click <strong>Authenticate</strong>.</li>
  </ol>
  <div class="row" style="margin:.75rem 0">
    <code id="redirectUri" style="flex:1;padding:.6rem .75rem">http://127.0.0.1:43827/callback</code>
    <button type="button" onclick="copyRedirectUri()">Copy URI</button>
  </div>
  <label class="label">Client ID</label>
  <input id="clientId" type="text" placeholder="Paste your Spotify Client ID" style="width:100%;background:#111;border:1px solid var(--border);color:var(--text);padding:.6rem;border-radius:8px;outline:none">
  <div style="margin-top:.75rem" class="row">
    <button id="manualAuthBtn" onclick="startAuth()">Authenticate</button>
  </div>
  <div id="authResult" style="margin-top:1rem"></div>
  <p style="font-size:.8rem;color:var(--muted);margin-top:.75rem">
    <strong>Tip:</strong> If the browser shows "Unable to connect" to <code>127.0.0.1:43827</code>
    after approving Spotify, your browser may be a Snap/Flatpak package that blocks localhost.
    Try Chrome/Chromium installed from a <code>.deb</code> package, or use the system default browser.
  </p>
</div>
<div class="card" style="opacity:.9">
  <h2>Auto Import from Browser (often fails)</h2>
  <p style="font-size:.9rem;color:var(--muted)">
    This reads the Client ID stored in your browser by open.spotify.com. That ID belongs
    to Spotify's official web player, so <strong>you cannot add a custom Redirect URI to it</strong>.
    Most users should register their own app above instead.
  </p>
  <label class="label">Select Browser</label>
  <div class="row">
    <select id="browserSelect" style="background:#111;border:1px solid var(--border);color:var(--text);padding:.5rem;border-radius:8px;outline:none;">
      <option value="chrome">Chrome</option>
      <option value="chromium">Chromium</option>
      <option value="firefox">Firefox</option>
      <option value="edge">Edge</option>
      <option value="brave">Brave</option>
      <option value="opera">Opera</option>
      <option value="vivaldi">Vivaldi</option>
      <option value="whale">Whale</option>
    </select>
    <button id="importBtn" onclick="importFromBrowser()">Import &amp; Authenticate</button>
  </div>
  <div id="importResult" style="margin-top:.75rem"></div>
</div>
<script>
async function copyRedirectUri(){
  const uri = document.getElementById("redirectUri").textContent;
  try {
    await navigator.clipboard.writeText(uri);
    const btn = event.target;
    const old = btn.textContent;
    btn.textContent = "Copied!";
    setTimeout(()=>btn.textContent=old, 1200);
  } catch(e) {
    alert("Copy failed: "+e);
  }
}
async function importFromBrowser(){
  const browser = document.getElementById("browserSelect").value;
  const box = document.getElementById("importResult");
  const btn = document.getElementById("importBtn");
  btn.disabled = true; btn.textContent = "Extracting…";
  box.innerHTML = 'Extracting Client ID from '+browser+'…';
  try {
    const res = await post("/login/extract", {browser: browser});
    btn.disabled = false; btn.textContent = "Import & Authenticate";
    if(res.error){ box.innerHTML = '<span style="color:var(--accent)">Error: '+res.error+'</span>'; return; }
    box.innerHTML = '<p><strong style="color:var(--accent)">Browser window opened.</strong></p>'+
      '<p>Approve Spotify access in the browser, then click <strong>Complete</strong>.</p>'+
      '<div style="margin-top:.5rem"><button onclick="completeAuth(\'importResult\')">Complete</button></div>';
    pollStatus();
  } catch(e) {
    box.innerHTML = '<span style="color:var(--accent)">Error: '+e+'</span>';
    btn.disabled = false; btn.textContent = "Import & Authenticate";
  }
}
async function startAuth(){
  const val = document.getElementById("clientId").value.trim();
  const box = document.getElementById("authResult");
  const btn = document.getElementById("manualAuthBtn");
  if(!val){ box.innerHTML = '<span style="color:var(--accent)">Please paste a Client ID.</span>'; return; }
  btn.disabled = true; btn.textContent = "Starting…";
  box.innerHTML = "Opening browser…";
  try {
    const res = await post("/login/initiate", {client_id: val});
    btn.disabled = false; btn.textContent = "Authenticate";
    if(res.error){ box.innerHTML = '<span style="color:var(--accent)">Error: '+res.error+'</span>'; return; }
    box.innerHTML = '<p><strong style="color:var(--accent)">Browser window opened.</strong></p>'+
      '<p>Approve Spotify access in the browser, then click <strong>Complete</strong>.</p>'+
      '<div style="margin-top:.5rem"><button onclick="completeAuth(\'authResult\')">Complete</button></div>';
    pollStatus();
  } catch(e) {
    box.innerHTML = '<span style="color:var(--accent)">Error: '+e+'</span>';
    btn.disabled=false; btn.textContent="Authenticate";
  }
}
async function completeAuth(boxId){
  const box = document.getElementById(boxId);
  box.innerHTML = "Completing…";
  try {
    const res = await post("/login/complete", {});
    if(res.error){ box.innerHTML = '<span style="color:var(--accent)">Error: '+res.error+'</span>'; return; }
    box.innerHTML = '<span style="color:var(--accent)">Authenticated successfully!</span>';
    setTimeout(()=>location.href="/", 1200);
  } catch(e) {
    box.innerHTML = '<span style="color:var(--accent)">Error: '+e+'</span>';
  }
}
async function pollStatus(){
  const check = async () => {
    try {
      const res = await fetch("/api/status");
      const data = await res.json();
      if(data.auth_status === "AUTH_STATUS_AUTHENTICATED"){
        document.getElementById("authResult").innerHTML = '<span style="color:var(--accent)">Authenticated successfully!</span>';
        setTimeout(()=>location.href="/", 1200);
        return;
      }
    } catch(e){}
    setTimeout(check, 3000);
  };
  check();
}
</script>
`
}

func render(content string) string {
	return fmt.Sprintf(`<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>gozik Spotify Plugin</title>
<style>
:root{--bg:#121212;--card:#181818;--text:#ffffff;--muted:#b3b3b3;--accent:#1db954;--accent-hover:#1ed760;--border:#282828;}
*{box-sizing:border-box}
body{margin:0;font-family:system-ui,-apple-system,BlinkMacSystemFont,"Segoe UI",Roboto,sans-serif;background:var(--bg);color:var(--text);display:flex;justify-content:center;padding:2rem 1rem}
.container{width:100%%;max-width:640px}
h1{margin:0 0 .25rem;font-size:1.6rem}
.sub{color:var(--muted);margin-bottom:1.5rem}
.card{background:var(--card);border:1px solid var(--border);border-radius:12px;padding:1.25rem;margin-bottom:1rem}
.card h2{margin:0 0 .75rem;font-size:1.1rem}
.status{display:inline-flex;align-items:center;gap:.5rem;padding:.35rem .7rem;border-radius:999px;font-size:.85rem;font-weight:600}
.status.ok{background:rgba(29,185,84,.15);color:var(--accent)}
.status.no{background:rgba(255,0,51,.15);color:#ff0033}
pre{background:#111;border:1px solid var(--border);border-radius:8px;padding:.75rem;overflow-x:auto;font-size:.9rem;color:var(--text)}
code{font-family:ui-monospace,SFMono-Regular,Menlo,Monaco,Consolas,monospace;font-size:.9rem;background:#111;padding:.2rem .4rem;border-radius:4px}
button{cursor:pointer;border:none;border-radius:8px;padding:.6rem 1.2rem;font-size:.9rem;font-weight:600;background:var(--accent);color:#000;transition:opacity .15s}
button:hover{opacity:.9;background:var(--accent-hover)}
button:disabled{opacity:.5;cursor:not-allowed}
textarea,input[type="text"]{width:100%%;min-height:48px;background:#111;border:1px solid var(--border);border-radius:8px;padding:.6rem .75rem;color:var(--text);font-family:ui-monospace,monospace;font-size:.9rem;outline:none}
textarea{min-height:120px;resize:vertical}
.label{display:block;margin-bottom:.35rem;font-size:.85rem;color:var(--muted)}
.row{display:flex;gap:.75rem;flex-wrap:wrap;align-items:center}
.footer{text-align:center;color:var(--muted);font-size:.8rem;margin-top:1rem}
a{color:var(--accent)}
a:hover{color:var(--accent-hover)}
hr{border:0;border-top:1px solid var(--border);margin:1rem 0}
</style>
</head>
<body>
<div class="container">
  <h1>🎵 gozik Spotify</h1>
  <p class="sub">Standalone plugin web console</p>
  %s
  <p class="footer">gozik-spotify gRPC plugin &middot; <a href="/api/status">Status JSON</a></p>
</div>
<script>
async function post(url,body){const r=await fetch(url,{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify(body)});return r.json();}
</script>
</body>
</html>
`, content)
}

func ifElse(cond bool, a, b string) string {
	if cond {
		return a
	}
	return b
}

// DesktopFilePath returns the Linux desktop entry path for the web UI.
func DesktopFilePath() string {
	if d := os.Getenv("XDG_DATA_HOME"); d != "" {
		return filepath.Join(d, "applications", "gozik-spotify-webui.desktop")
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".local", "share", "applications", "gozik-spotify-webui.desktop")
}
