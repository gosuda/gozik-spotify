// Package spotify contains a minimal Spotify Web API client.
package spotify

// Image is a Spotify image object.
type Image struct {
	URL    string `json:"url"`
	Height int    `json:"height"`
	Width  int    `json:"width"`
}

// Artist is a Spotify artist object.
type Artist struct {
	ID     string  `json:"id"`
	Name   string  `json:"name"`
	Images []Image `json:"images"`
}

// Album is a Spotify album object.
type Album struct {
	ID         string  `json:"id"`
	Name       string  `json:"name"`
	Artists    []Artist `json:"artists"`
	Images     []Image `json:"images"`
	ReleaseDate string `json:"release_date"`
	TotalTracks int    `json:"total_tracks"`
}

// Track is a Spotify track object.
type Track struct {
	ID           string   `json:"id"`
	Name         string   `json:"name"`
	Artists      []Artist `json:"artists"`
	Album        Album    `json:"album"`
	DurationMs   int      `json:"duration_ms"`
	Explicit     bool     `json:"explicit"`
	ExternalIDs  struct {
		ISRC string `json:"isrc"`
	} `json:"external_ids"`
	URI string `json:"uri"`
}

// Playlist is a Spotify playlist object.
type Playlist struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Owner       struct {
		DisplayName string `json:"display_name"`
	} `json:"owner"`
	Images      []Image `json:"images"`
	Tracks      struct {
		Total int `json:"total"`
		Items []struct {
			Track Track `json:"track"`
		} `json:"items"`
		Next string `json:"next"`
	} `json:"tracks"`
}

// Paging is the Spotify paging object wrapper.
type Paging[T any] struct {
	Items    []T    `json:"items"`
	Next     string `json:"next"`
	Offset   int    `json:"offset"`
	Total    int    `json:"total"`
	Limit    int    `json:"limit"`
}

// SearchResult holds the typed results from the search endpoint.
type SearchResult struct {
	Tracks   Paging[Track]   `json:"tracks"`
	Albums   Paging[Album]   `json:"albums"`
	Artists  Paging[Artist]  `json:"artists"`
	Playlists Paging[Playlist] `json:"playlists"`
}

// User represents the current user (only the fields we need).
type User struct {
	ID          string `json:"id"`
	DisplayName string `json:"display_name"`
	Product     string `json:"product"`
}
