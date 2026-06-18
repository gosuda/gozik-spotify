package spotify

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const baseURL = "https://api.spotify.com/v1"

// Client is a thin HTTP client for the Spotify Web API.
type Client struct {
	http       *http.Client
	accessToken string
}

// NewClient creates a client from an access token.
func NewClient(token string) *Client {
	return &Client{
		http:        &http.Client{Timeout: 30 * time.Second},
		accessToken: token,
	}
}

// SetToken updates the access token used for requests.
func (c *Client) SetToken(token string) {
	c.accessToken = token
}

func (c *Client) request(ctx context.Context, method, path string, params url.Values, body io.Reader, out any) error {
	u := baseURL + path
	if len(params) > 0 {
		u += "?" + params.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, method, u, body)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.accessToken)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("spotify request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read spotify response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("spotify status %d: %s", resp.StatusCode, string(respBody))
	}
	if out != nil {
		if err := json.Unmarshal(respBody, out); err != nil {
			return fmt.Errorf("parse spotify response: %w", err)
		}
	}
	return nil
}

// Search queries the Spotify search endpoint for the requested types.
func (c *Client) Search(ctx context.Context, query string, types []string, limit, offset int) (SearchResult, error) {
	var result SearchResult
	if limit <= 0 {
		limit = 20
	}
	params := url.Values{}
	params.Set("q", query)
	params.Set("type", strings.Join(types, ","))
	params.Set("limit", strconv.Itoa(limit))
	params.Set("offset", strconv.Itoa(offset))
	if err := c.request(ctx, http.MethodGet, "/search", params, nil, &result); err != nil {
		return result, err
	}
	return result, nil
}

// GetTrack fetches a single track by Spotify ID.
func (c *Client) GetTrack(ctx context.Context, id string) (Track, error) {
	var track Track
	if err := c.request(ctx, http.MethodGet, "/tracks/"+id, nil, nil, &track); err != nil {
		return track, err
	}
	return track, nil
}

// GetPlaylist fetches playlist metadata.
func (c *Client) GetPlaylist(ctx context.Context, id string) (Playlist, error) {
	var p Playlist
	if err := c.request(ctx, http.MethodGet, "/playlists/"+id, nil, nil, &p); err != nil {
		return p, err
	}
	return p, nil
}

// GetPlaylistTracks fetches tracks inside a playlist, following pagination up to limit.
func (c *Client) GetPlaylistTracks(ctx context.Context, id string, limit int) ([]Track, error) {
	if limit <= 0 {
		limit = 100
	}
	var tracks []Track
	offset := 0
	for len(tracks) < limit {
		params := url.Values{}
		params.Set("limit", strconv.Itoa(min(50, limit-len(tracks))))
		params.Set("offset", strconv.Itoa(offset))
		var page Paging[struct {
			Track Track `json:"track"`
		}]
		if err := c.request(ctx, http.MethodGet, "/playlists/"+id+"/tracks", params, nil, &page); err != nil {
			return nil, err
		}
		for _, item := range page.Items {
			if item.Track.ID != "" {
				tracks = append(tracks, item.Track)
			}
		}
		if page.Next == "" || len(page.Items) == 0 {
			break
		}
		offset += len(page.Items)
	}
	return tracks, nil
}

// GetUserPlaylists returns the current user's playlists.
func (c *Client) GetUserPlaylists(ctx context.Context, limit int) ([]Playlist, error) {
	if limit <= 0 {
		limit = 50
	}
	var out []Playlist
	offset := 0
	for len(out) < limit {
		params := url.Values{}
		params.Set("limit", strconv.Itoa(min(50, limit-len(out))))
		params.Set("offset", strconv.Itoa(offset))
		var page Paging[Playlist]
		if err := c.request(ctx, http.MethodGet, "/me/playlists", params, nil, &page); err != nil {
			return nil, err
		}
		out = append(out, page.Items...)
		if page.Next == "" || len(page.Items) == 0 {
			break
		}
		offset += len(page.Items)
	}
	return out, nil
}

// GetCurrentUser fetches the current user's profile.
func (c *Client) GetCurrentUser(ctx context.Context) (User, error) {
	var user User
	if err := c.request(ctx, http.MethodGet, "/me", nil, nil, &user); err != nil {
		return user, err
	}
	return user, nil
}

// GetUserLibrary returns the current user's liked tracks.
func (c *Client) GetUserLibrary(ctx context.Context, limit int) ([]Track, error) {
	if limit <= 0 {
		limit = 50
	}
	var out []Track
	offset := 0
	for len(out) < limit {
		params := url.Values{}
		params.Set("limit", strconv.Itoa(min(50, limit-len(out))))
		params.Set("offset", strconv.Itoa(offset))
		var page Paging[struct {
			Track Track `json:"track"`
		}]
		if err := c.request(ctx, http.MethodGet, "/me/tracks", params, nil, &page); err != nil {
			return nil, err
		}
		for _, item := range page.Items {
			if item.Track.ID != "" {
				out = append(out, item.Track)
			}
		}
		if page.Next == "" || len(page.Items) == 0 {
			break
		}
		offset += len(page.Items)
	}
	return out, nil
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
