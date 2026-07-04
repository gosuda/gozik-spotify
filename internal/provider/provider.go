package provider

import (
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	musicv1 "github.com/gg582/gozik/api/music/v1"
	"github.com/gg582/gozik-spotify/internal/auth"
	"github.com/gg582/gozik-spotify/internal/browserid"
	"github.com/gg582/gozik-spotify/internal/config"
	"github.com/gg582/gozik-spotify/internal/spotify"
	"github.com/gg582/gozik-spotify/internal/stream"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// fallbackClientID can be injected at build time with
// -ldflags "-X github.com/gg582/gozik-spotify/internal/provider.fallbackClientID=..."
var fallbackClientID string

const (
	providerID   = "spotify"
	displayName  = "Spotify"
	defaultLimit = 10
)

// MusicProviderServicer implements music.v1.MusicProviderService.
type MusicProviderServicer struct {
	musicv1.UnimplementedMusicProviderServiceServer

	clientID string
	resolver stream.Resolver

	mu       sync.RWMutex
	client   *spotify.Client
	tokens   *config.TokenStore
	pending  *auth.Flow
}

// New creates a servicer, loading persisted tokens if available.
func New() (*MusicProviderServicer, error) {
	clientID := os.Getenv("GOZIK_SPOTIFY_CLIENT_ID")
	if clientID == "" {
		if s, err := config.LoadSettings(); err == nil && s != nil {
			clientID = s.ClientID
		}
	}

	resolver, err := stream.New()
	if err != nil {
		return nil, fmt.Errorf("stream resolver: %w", err)
	}

	s := &MusicProviderServicer{
		clientID: clientID,
		resolver: resolver,
	}

	if ts, err := config.LoadTokens(); err != nil {
		log.Printf("failed to load tokens: %v", err)
	} else if ts != nil && ts.AccessToken != "" {
		s.tokens = ts
		s.client = spotify.NewClient(ts.AccessToken)
	}

	return s, nil
}

func (s *MusicProviderServicer) authStatus() musicv1.AuthStatus {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.tokens == nil || s.tokens.AccessToken == "" {
		return musicv1.AuthStatus_AUTH_STATUS_UNAUTHENTICATED
	}
	if time.Now().After(time.Unix(s.tokens.ExpiresAt, 0)) {
		return musicv1.AuthStatus_AUTH_STATUS_EXPIRED
	}
	return musicv1.AuthStatus_AUTH_STATUS_AUTHENTICATED
}

// GetProviderMetadata returns static provider metadata and current auth status.
func (s *MusicProviderServicer) GetProviderMetadata(ctx context.Context, _ *musicv1.GetProviderMetadataRequest) (*musicv1.GetProviderMetadataResponse, error) {
	return &musicv1.GetProviderMetadataResponse{
		ProviderId:   providerID,
		DisplayName:  displayName,
		Capabilities: []musicv1.ProviderCapability{
			musicv1.ProviderCapability_PROVIDER_CAPABILITY_SEARCH,
			musicv1.ProviderCapability_PROVIDER_CAPABILITY_STREAM_TRACK,
			musicv1.ProviderCapability_PROVIDER_CAPABILITY_LIBRARY_MANAGEMENT,
		},
		AuthStatus: s.authStatus(),
		AuthUrl:    "https://accounts.spotify.com/authorize",
	}, nil
}

// InitiateAuth starts the PKCE loopback flow and opens the browser.
// The caller may pass client_id in req.Params["client_id"] so the front-end
// can collect it at runtime, or pass browser in req.Params["browser"] to
// extract a Client ID from local browser storage (yt-dlp-style).
func (s *MusicProviderServicer) InitiateAuth(ctx context.Context, req *musicv1.InitiateAuthRequest) (*musicv1.InitiateAuthResponse, error) {
	clientID := ""
	if req != nil {
		clientID = strings.TrimSpace(req.GetParams()["client_id"])
	}

	// Tier 1: explicit client id from the front-end or environment.
	if clientID != "" {
		if err := config.SaveSettings(&config.ProviderSettings{ClientID: clientID}); err != nil {
			return nil, status.Errorf(codes.Internal, "save client id: %v", err)
		}
		s.clientID = clientID
	}

	// Tier 2: auto-detect from a named browser.
	if clientID == "" && req != nil {
		if browser := strings.TrimSpace(req.GetParams()["browser"]); browser != "" {
			res, err := browserid.Extract(browser)
			if err != nil {
				log.Printf("InitiateAuth: browser extraction from %q failed: %v", browser, err)
				return nil, status.Errorf(codes.FailedPrecondition,
					"Could not extract a Spotify Client ID from %q. Register your own app at "+
						"https://developer.spotify.com/dashboard and add http://127.0.0.1:43827/callback as a redirect URI.", browser)
			}
			if err := auth.ValidateClientID(res.ClientID); err != nil {
				log.Printf("InitiateAuth: extracted client id %q from %q rejected: %v", res.ClientID, browser, err)
				return nil, status.Errorf(codes.FailedPrecondition,
					"The Client ID extracted from %q is not usable: %v. "+
						"Register your own app at https://developer.spotify.com/dashboard "+
						"and add http://127.0.0.1:43827/callback as a redirect URI.", browser, err)
			}
			clientID = res.ClientID
			log.Printf("InitiateAuth: using client id extracted from %q (%s)", browser, res.Source)
		}
	}

	// Tier 3: build-time fallback Client ID.
	if clientID == "" && fallbackClientID != "" {
		if err := auth.ValidateClientID(fallbackClientID); err != nil {
			log.Printf("InitiateAuth: fallback client id rejected: %v", err)
		} else {
			clientID = fallbackClientID
			log.Printf("InitiateAuth: using build-time fallback client id")
		}
	}

	// Tier 4: already-persisted settings / environment variable.
	if clientID == "" {
		clientID = s.clientID
	}

	if clientID == "" {
		return nil, status.Errorf(codes.FailedPrecondition,
			"Spotify client ID is not configured. "+
				"Pass client_id in InitiateAuth params, choose a browser for auto-import, "+
				"set GOZIK_SPOTIFY_CLIENT_ID, or build with a fallback Client ID.")
	}

	s.clientID = clientID

	preferredBrowser := ""
	if req != nil {
		preferredBrowser = strings.TrimSpace(req.GetParams()["browser"])
	}

	flow, err := auth.NewFlow(s.clientID)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "auth flow: %v", err)
	}
	flow.SetPreferredBrowser(preferredBrowser)

	s.mu.Lock()
	s.pending = flow
	s.mu.Unlock()

	if err := flow.StartServer(); err != nil {
		return nil, status.Errorf(codes.Internal, "start auth server: %v", err)
	}

	log.Printf("InitiateAuth: browser opened for Spotify PKCE flow")
	return &musicv1.InitiateAuthResponse{
		AuthUrl:    flow.AuthURL(),
		DeviceCode: flow.DeviceCode(),
	}, nil
}

// CompleteAuth waits for the loopback callback and returns the stored tokens.
func (s *MusicProviderServicer) CompleteAuth(ctx context.Context, _ *musicv1.CompleteAuthRequest) (*musicv1.CompleteAuthResponse, error) {
	s.mu.Lock()
	flow := s.pending
	s.pending = nil
	s.mu.Unlock()

	if flow == nil {
		return nil, status.Errorf(codes.FailedPrecondition, "no authentication flow is pending; call InitiateAuth first")
	}

	ts, err := flow.Result(ctx)
	if err != nil {
		return nil, status.Errorf(codes.Unauthenticated, "authentication failed: %v", err)
	}

	s.mu.Lock()
	s.tokens = ts
	s.client = spotify.NewClient(ts.AccessToken)
	if flow.ClientID() != "" {
		s.clientID = flow.ClientID()
	}
	s.mu.Unlock()

	if flow.ClientID() != "" {
		if err := config.SaveSettings(&config.ProviderSettings{ClientID: flow.ClientID()}); err != nil {
			log.Printf("CompleteAuth: failed to save client id to settings: %v", err)
		}
	}

	// Verify the authenticated account and log the subscription tier so
	// "premium required" problems are easier to diagnose.
	go func(token string) {
		probeCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if user, err := spotify.NewClient(token).GetCurrentUser(probeCtx); err == nil {
			log.Printf("CompleteAuth: logged in as %q (%s)", user.DisplayName, user.Product)
		}
	}(ts.AccessToken)

	return &musicv1.CompleteAuthResponse{
		AccessToken:  ts.AccessToken,
		RefreshToken: ts.RefreshToken,
		ExpiresAt:    ts.ExpiresAt * 1000,
	}, nil
}

// RefreshAuth refreshes the access token using the stored refresh token.
func (s *MusicProviderServicer) RefreshAuth(ctx context.Context, _ *musicv1.RefreshAuthRequest) (*musicv1.RefreshAuthResponse, error) {
	s.mu.RLock()
	ts := s.tokens
	clientID := s.clientID
	s.mu.RUnlock()

	if ts == nil || ts.RefreshToken == "" {
		return nil, status.Errorf(codes.Unauthenticated, "no refresh token available; authenticate first")
	}
	if clientID == "" {
		return nil, status.Errorf(codes.FailedPrecondition, "client ID required for token refresh")
	}

	refreshed, err := auth.RefreshAccessToken(clientID, ts.RefreshToken)
	if err != nil {
		return nil, status.Errorf(codes.Unauthenticated, "token refresh failed: %v", err)
	}

	s.mu.Lock()
	s.tokens = refreshed
	if s.client != nil {
		s.client.SetToken(refreshed.AccessToken)
	} else {
		s.client = spotify.NewClient(refreshed.AccessToken)
	}
	s.mu.Unlock()

	return &musicv1.RefreshAuthResponse{
		AccessToken: refreshed.AccessToken,
		ExpiresAt:   refreshed.ExpiresAt * 1000,
	}, nil
}

// Search queries Spotify for tracks, albums, artists, and playlists.
func (s *MusicProviderServicer) Search(ctx context.Context, req *musicv1.SearchRequest) (*musicv1.SearchResponse, error) {
	query := strings.TrimSpace(req.Query)
	if query == "" {
		return nil, status.Errorf(codes.InvalidArgument, "query must not be empty")
	}

	limit := int(req.Limit)
	if limit <= 0 {
		limit = defaultLimit
	}

	types := s.searchTypes(req.Types)
	if len(types) == 0 {
		types = []string{"track", "album", "artist", "playlist"}
	}

	client, err := s.apiClient(ctx)
	if err != nil {
		return nil, err
	}

	result, err := client.Search(ctx, query, types, limit, 0)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "spotify search failed: %v", err)
	}

	resp := &musicv1.SearchResponse{NextPageToken: ""}
	for _, t := range result.Tracks.Items {
		resp.Tracks = append(resp.Tracks, toProtoTrack(t))
	}
	for _, a := range result.Albums.Items {
		resp.Albums = append(resp.Albums, toProtoAlbum(a))
	}
	for _, ar := range result.Artists.Items {
		resp.Artists = append(resp.Artists, toProtoArtist(ar))
	}
	for _, p := range result.Playlists.Items {
		resp.Playlists = append(resp.Playlists, toProtoPlaylist(p))
	}

	log.Printf("Search('%s') → tracks=%d albums=%d artists=%d playlists=%d",
		query, len(resp.Tracks), len(resp.Albums), len(resp.Artists), len(resp.Playlists))
	return resp, nil
}

// SearchSuggestions is not supported by the Spotify Web API; returns empty.
func (s *MusicProviderServicer) SearchSuggestions(req *musicv1.SearchSuggestionsRequest, srv musicv1.MusicProviderService_SearchSuggestionsServer) error {
	return nil
}

// GetTrackDetails fetches detailed metadata for a Spotify track.
func (s *MusicProviderServicer) GetTrackDetails(ctx context.Context, req *musicv1.GetTrackDetailsRequest) (*musicv1.GetTrackDetailsResponse, error) {
	id := stripTrackID(req.TrackId)
	if id == "" {
		return nil, status.Errorf(codes.InvalidArgument, "track_id must not be empty")
	}
	client, err := s.apiClient(ctx)
	if err != nil {
		return nil, err
	}
	track, err := client.GetTrack(ctx, id)
	if err != nil {
		return nil, status.Errorf(codes.NotFound, "track not found: %v", err)
	}
	return &musicv1.GetTrackDetailsResponse{Track: toProtoTrack(track)}, nil
}

// ResolveStream returns a playable stream URL for a track.
func (s *MusicProviderServicer) ResolveStream(ctx context.Context, req *musicv1.ResolveStreamRequest) (*musicv1.ResolveStreamResponse, error) {
	track, err := s.GetTrackDetails(ctx, &musicv1.GetTrackDetailsRequest{TrackId: req.TrackId})
	if err != nil {
		return nil, err
	}

	url, headers, expiry, err := s.resolver.Resolve(ctx, track.Track)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "resolve stream failed: %v", err)
	}

	log.Printf("ResolveStream: %s → %d bytes URL, expiry=%d", req.TrackId, len(url), expiry)
	return &musicv1.ResolveStreamResponse{
		StreamUrl: url,
		Headers:   headers,
		ExpiryMs:  expiry,
	}, nil
}

// StreamAudio streams raw audio bytes over gRPC with Range-based resume.
func (s *MusicProviderServicer) StreamAudio(req *musicv1.StreamAudioRequest, srv musicv1.MusicProviderService_StreamAudioServer) error {
	ctx := srv.Context()
	track, err := s.GetTrackDetails(ctx, &musicv1.GetTrackDetailsRequest{TrackId: req.TrackId})
	if err != nil {
		return err
	}

	url, headers, _, err := s.resolver.Resolve(ctx, track.Track)
	if err != nil {
		return status.Errorf(codes.Internal, "resolve stream failed: %v", err)
	}

	const chunkSize = 64 * 1024
	const maxRetries = 3
	startMs := req.StartPositionMs

	// Estimate byte offset from time using a conservative bitrate.
	bitrateKbps := 128.0
	byteOffset := int(startMs * int64(bitrateKbps) / 8)
	bytesStreamed := 0
	timestampMs := startMs

	for attempt := 0; attempt <= maxRetries; attempt++ {
		if err := ctx.Err(); err != nil {
			return ctx.Err()
		}

		if attempt > 0 {
			log.Printf("StreamAudio: reconnect attempt %d/%d for %s at byte %d", attempt, maxRetries, req.TrackId, byteOffset)
			time.Sleep(minDuration(time.Duration(attempt)*time.Second, 10*time.Second))
		}

		httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			return status.Errorf(codes.Internal, "build request: %v", err)
		}
		for k, v := range headers {
			httpReq.Header.Set(k, v)
		}
		if byteOffset > 0 {
			httpReq.Header.Set("Range", fmt.Sprintf("bytes=%d-", byteOffset))
		}

		resp, err := http.DefaultClient.Do(httpReq)
		if err != nil {
			if attempt < maxRetries {
				continue
			}
			return status.Errorf(codes.Internal, "audio request failed: %v", err)
		}

		if byteOffset > 0 && resp.StatusCode != http.StatusPartialContent {
			resp.Body.Close()
			return status.Errorf(codes.Internal, "server ignored Range request (status %d)", resp.StatusCode)
		}

		buf := make([]byte, chunkSize)
		for {
			if err := ctx.Err(); err != nil {
				resp.Body.Close()
				return ctx.Err()
			}
			n, err := resp.Body.Read(buf)
			if n > 0 {
				chunk := make([]byte, n)
				copy(chunk, buf[:n])
				if err := srv.Send(&musicv1.AudioChunk{
					Data:        chunk,
					TimestampMs: timestampMs,
					Eof:         false,
				}); err != nil {
					resp.Body.Close()
					return err
				}
				bytesStreamed += n
				byteOffset += n
				timestampMs = startMs + int64(float64(bytesStreamed)*8.0/bitrateKbps)
			}
			if err == io.EOF {
				resp.Body.Close()
				_ = srv.Send(&musicv1.AudioChunk{Data: []byte{}, TimestampMs: timestampMs, Eof: true})
				return nil
			}
			if err != nil {
				resp.Body.Close()
				if attempt < maxRetries {
					break
				}
				return status.Errorf(codes.Internal, "audio stream read error: %v", err)
			}
		}
	}

	return status.Errorf(codes.Internal, "audio stream exhausted retries")
}

// GetUserLibrary returns the authenticated user's liked tracks.
func (s *MusicProviderServicer) GetUserLibrary(ctx context.Context, req *musicv1.GetUserLibraryRequest) (*musicv1.GetUserLibraryResponse, error) {
	client, err := s.requireAuth(ctx)
	if err != nil {
		return nil, err
	}
	limit := int(req.Limit)
	if limit <= 0 {
		limit = 50
	}
	tracks, err := client.GetUserLibrary(ctx, limit)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to fetch library: %v", err)
	}
	out := make([]*musicv1.Track, 0, len(tracks))
	for _, t := range tracks {
		out = append(out, toProtoTrack(t))
	}
	return &musicv1.GetUserLibraryResponse{Tracks: out, NextPageToken: ""}, nil
}

// GetUserPlaylists returns the authenticated user's playlists.
func (s *MusicProviderServicer) GetUserPlaylists(ctx context.Context, req *musicv1.GetUserPlaylistsRequest) (*musicv1.GetUserPlaylistsResponse, error) {
	client, err := s.requireAuth(ctx)
	if err != nil {
		return nil, err
	}
	limit := int(req.Limit)
	if limit <= 0 {
		limit = 50
	}
	playlists, err := client.GetUserPlaylists(ctx, limit)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to fetch playlists: %v", err)
	}
	out := make([]*musicv1.Playlist, 0, len(playlists))
	for _, p := range playlists {
		out = append(out, toProtoPlaylist(p))
	}
	return &musicv1.GetUserPlaylistsResponse{Playlists: out, NextPageToken: ""}, nil
}

// GetPlaylistDetails returns metadata and tracks for a Spotify playlist.
func (s *MusicProviderServicer) GetPlaylistDetails(ctx context.Context, req *musicv1.GetPlaylistDetailsRequest) (*musicv1.GetPlaylistDetailsResponse, error) {
	id := strings.TrimSpace(req.PlaylistId)
	if id == "" {
		return nil, status.Errorf(codes.InvalidArgument, "playlist_id must not be empty")
	}
	client, err := s.apiClient(ctx)
	if err != nil {
		return nil, err
	}

	limit := int(req.Limit)
	if limit <= 0 {
		limit = 100
	}

	playlist, err := client.GetPlaylist(ctx, id)
	if err != nil {
		return nil, status.Errorf(codes.NotFound, "playlist not found: %v", err)
	}
	tracks, err := client.GetPlaylistTracks(ctx, id, limit)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to fetch playlist tracks: %v", err)
	}

	out := make([]*musicv1.Track, 0, len(tracks))
	for _, t := range tracks {
		out = append(out, toProtoTrack(t))
	}
	return &musicv1.GetPlaylistDetailsResponse{
		Playlist:      toProtoPlaylist(playlist),
		Tracks:        out,
		NextPageToken: "",
	}, nil
}

func (s *MusicProviderServicer) searchTypes(types []musicv1.MediaType) []string {
	if len(types) == 0 {
		return nil
	}
	m := map[musicv1.MediaType]string{
		musicv1.MediaType_MEDIA_TYPE_TRACK:    "track",
		musicv1.MediaType_MEDIA_TYPE_ALBUM:    "album",
		musicv1.MediaType_MEDIA_TYPE_ARTIST:   "artist",
		musicv1.MediaType_MEDIA_TYPE_PLAYLIST: "playlist",
	}
	var out []string
	seen := make(map[string]struct{})
	for _, t := range types {
		if v, ok := m[t]; ok {
			if _, exists := seen[v]; !exists {
				seen[v] = struct{}{}
				out = append(out, v)
			}
		}
	}
	return out
}

func (s *MusicProviderServicer) apiClient(ctx context.Context) (*spotify.Client, error) {
	s.mu.RLock()
	client := s.client
	tokens := s.tokens
	s.mu.RUnlock()

	if client == nil {
		// Search can work without auth for public catalogs in some regions, but
		// Spotify requires an access token even for search. Return a clear error.
		return nil, status.Errorf(codes.Unauthenticated, "not authenticated with Spotify")
	}

	if tokens != nil && time.Now().After(time.Unix(tokens.ExpiresAt, 0).Add(-time.Minute)) {
		// Token expired or about to expire; try refresh.
		// Do NOT hold s.mu while calling RefreshAuth: it needs a write lock and
		// would deadlock with the RLock above.
		if _, err := s.RefreshAuth(ctx, &musicv1.RefreshAuthRequest{}); err != nil {
			return nil, status.Errorf(codes.Unauthenticated, "Spotify token expired and refresh failed: %v", err)
		}
		s.mu.RLock()
		client = s.client
		s.mu.RUnlock()
	}

	return client, nil
}

func (s *MusicProviderServicer) requireAuth(ctx context.Context) (*spotify.Client, error) {
	if s.authStatus() != musicv1.AuthStatus_AUTH_STATUS_AUTHENTICATED {
		return nil, status.Errorf(codes.Unauthenticated, "this operation requires Spotify authentication")
	}
	return s.apiClient(ctx)
}

func minDuration(a, b time.Duration) time.Duration {
	if a < b {
		return a
	}
	return b
}
