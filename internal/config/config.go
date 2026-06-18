// Package config handles persistence for Spotify tokens and plugin settings.
package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// TokenStore holds OAuth tokens returned by Spotify's token endpoint.
type TokenStore struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	TokenType    string `json:"token_type"`
	ExpiresAt    int64  `json:"expires_at"`
}

// ProviderSettings holds non-secret plugin configuration.
type ProviderSettings struct {
	ClientID string `json:"client_id"`
}

// Dir returns the host OS standard configuration directory for gozik.
func Dir() string {
	if d := os.Getenv("XDG_CONFIG_HOME"); d != "" {
		return filepath.Join(d, "gozik")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		home = "."
	}
	return filepath.Join(home, ".config", "gozik")
}

// TokenPath returns the path to the persisted token file.
func TokenPath() string {
	return filepath.Join(Dir(), "spotify_tokens.json")
}

// SettingsPath returns the path to the persisted settings file.
func SettingsPath() string {
	return filepath.Join(Dir(), "spotify_settings.json")
}

// LoadTokens reads saved tokens from disk, returning nil when no file exists.
func LoadTokens() (*TokenStore, error) {
	path := TokenPath()
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read token file: %w", err)
	}
	var ts TokenStore
	if err := json.Unmarshal(data, &ts); err != nil {
		return nil, fmt.Errorf("parse token file: %w", err)
	}
	return &ts, nil
}

// SaveTokens persists tokens, creating the config directory if needed.
func SaveTokens(ts *TokenStore) error {
	if err := os.MkdirAll(Dir(), 0o700); err != nil {
		return fmt.Errorf("create config dir: %w", err)
	}
	data, err := json.MarshalIndent(ts, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal tokens: %w", err)
	}
	if err := os.WriteFile(TokenPath(), data, 0o600); err != nil {
		return fmt.Errorf("write token file: %w", err)
	}
	return nil
}

// LoadSettings reads persisted provider settings.
func LoadSettings() (*ProviderSettings, error) {
	data, err := os.ReadFile(SettingsPath())
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read settings file: %w", err)
	}
	var s ProviderSettings
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, fmt.Errorf("parse settings file: %w", err)
	}
	return &s, nil
}

// SaveSettings persists provider settings.
func SaveSettings(s *ProviderSettings) error {
	if err := os.MkdirAll(Dir(), 0o700); err != nil {
		return fmt.Errorf("create config dir: %w", err)
	}
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal settings: %w", err)
	}
	return os.WriteFile(SettingsPath(), data, 0o600)
}
