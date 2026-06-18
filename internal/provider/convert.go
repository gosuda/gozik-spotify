package provider

import (
	"fmt"
	"strconv"
	"strings"

	musicv1 "github.com/gg582/gozik/api/music/v1"
	"github.com/gg582/gozik-spotify/internal/spotify"
)

const trackIDPrefix = "spotify:track:"

// trackID returns the canonical provider track ID for a Spotify track.
func trackID(t spotify.Track) string {
	if t.URI != "" {
		return t.URI
	}
	return trackIDPrefix + t.ID
}

// stripTrackID extracts the raw Spotify track ID from a provider track ID.
func stripTrackID(id string) string {
	id = strings.TrimSpace(id)
	if strings.HasPrefix(id, trackIDPrefix) {
		return strings.TrimPrefix(id, trackIDPrefix)
	}
	return id
}

// toProtoTrack maps a Spotify track to the shared music provider Track message.
func toProtoTrack(t spotify.Track) *musicv1.Track {
	artists := make([]*musicv1.Artist, 0, len(t.Artists))
	for _, a := range t.Artists {
		artists = append(artists, &musicv1.Artist{Id: a.ID, Name: a.Name})
	}
	return &musicv1.Track{
		Id:         trackID(t),
		Title:      t.Name,
		Artists:    artists,
		Album:      toProtoAlbum(t.Album),
		DurationMs: int64(t.DurationMs),
		Images:     toProtoImages(t.Album.Images),
		Explicit:   t.Explicit,
	}
}

// toProtoAlbum maps a Spotify album to the shared Album message.
func toProtoAlbum(a spotify.Album) *musicv1.Album {
	if a.ID == "" && a.Name == "" {
		return nil
	}
	artists := make([]*musicv1.Artist, 0, len(a.Artists))
	for _, ar := range a.Artists {
		artists = append(artists, &musicv1.Artist{Id: ar.ID, Name: ar.Name})
	}
	year := 0
	if len(a.ReleaseDate) >= 4 {
		if y, err := strconv.Atoi(a.ReleaseDate[:4]); err == nil {
			year = y
		}
	}
	return &musicv1.Album{
		Id:         a.ID,
		Title:      a.Name,
		Artists:    artists,
		Images:     toProtoImages(a.Images),
		ReleaseYear: int32(year),
		TrackCount: int32(a.TotalTracks),
	}
}

// toProtoArtist maps a Spotify artist to the shared Artist message.
func toProtoArtist(a spotify.Artist) *musicv1.Artist {
	return &musicv1.Artist{Id: a.ID, Name: a.Name}
}

// toProtoPlaylist maps a Spotify playlist to the shared Playlist message.
func toProtoPlaylist(p spotify.Playlist) *musicv1.Playlist {
	return &musicv1.Playlist{
		Id:          p.ID,
		Title:       p.Name,
		Description: p.Description,
		OwnerName:   p.Owner.DisplayName,
		Images:      toProtoImages(p.Images),
		TrackCount:  int32(p.Tracks.Total),
	}
}

// toProtoImages maps Spotify images to shared Image messages.
func toProtoImages(images []spotify.Image) []*musicv1.Image {
	out := make([]*musicv1.Image, 0, len(images))
	for _, img := range images {
		out = append(out, &musicv1.Image{
			Url:    img.URL,
			Width:  int32(img.Width),
			Height: int32(img.Height),
		})
	}
	return out
}

// toSearchQuery builds a deterministic fallback audio search query.
// It is used by hybrid stream resolution to find a playable audio source.
func toSearchQuery(t *musicv1.Track) string {
	if t == nil {
		return ""
	}
	parts := []string{t.Title}
	for _, a := range t.Artists {
		if a.Name != "" {
			parts = append(parts, a.Name)
		}
	}
	parts = append(parts, "audio")
	return strings.Join(parts, " ")
}

// fmtTrackName returns a human-readable "Title - Artist" string.
func fmtTrackName(t *musicv1.Track) string {
	if t == nil {
		return ""
	}
	names := make([]string, 0, len(t.Artists))
	for _, a := range t.Artists {
		if a.Name != "" {
			names = append(names, a.Name)
		}
	}
	if len(names) == 0 {
		return t.Title
	}
	return fmt.Sprintf("%s - %s", t.Title, strings.Join(names, ", "))
}
