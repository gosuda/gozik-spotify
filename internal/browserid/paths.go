package browserid

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// chromiumConfig returns the base configuration directory for a Chromium-family browser.
func chromiumConfig(browser string) string {
	home, _ := os.UserHomeDir()
	switch runtime.GOOS {
	case "darwin":
		return filepath.Join(home, "Library", "Application Support", chromiumAppName(browser))
	case "windows":
		local := os.Getenv("LOCALAPPDATA")
		if local == "" {
			local = filepath.Join(home, "AppData", "Local")
		}
		return filepath.Join(local, chromiumAppName(browser))
	default: // linux and others
		config := os.Getenv("XDG_CONFIG_HOME")
		if config == "" {
			config = filepath.Join(home, ".config")
		}
		return filepath.Join(config, chromiumConfigName(browser))
	}
}

func chromiumAppName(browser string) string {
	switch browser {
	case Chrome:
		return "Google/Chrome"
	case Chromium:
		return "Chromium"
	case Edge:
		return "Microsoft Edge"
	case Brave:
		return "BraveSoftware/Brave-Browser"
	case Opera:
		return "com.operasoftware.Opera"
	case Vivaldi:
		return "Vivaldi"
	case Whale:
		return "Naver/Whale"
	default:
		return browser
	}
}

func chromiumConfigName(browser string) string {
	switch browser {
	case Chrome:
		return "google-chrome"
	case Chromium:
		return "chromium"
	case Edge:
		return "microsoft-edge"
	case Brave:
		return "BraveSoftware/Brave-Browser"
	case Opera:
		return "opera"
	case Vivaldi:
		return "vivaldi"
	case Whale:
		return "naver-whale"
	default:
		return browser
	}
}

// chromiumProfileDir returns the default profile directory.
func chromiumProfileDir(browser string) string {
	return filepath.Join(chromiumConfig(browser), "Default")
}

// chromiumLocalStorageDir returns the LevelDB local-storage directory.
func chromiumLocalStorageDir(browser string) string {
	return filepath.Join(chromiumProfileDir(browser), "Local Storage", "leveldb")
}

// firefoxProfileDir locates the default Firefox profile directory.
func firefoxProfileDir() (string, error) {
	home, _ := os.UserHomeDir()
	var base string
	switch runtime.GOOS {
	case "darwin":
		base = filepath.Join(home, "Library", "Application Support", "Firefox", "Profiles")
	case "windows":
		roaming := os.Getenv("APPDATA")
		if roaming == "" {
			roaming = filepath.Join(home, "AppData", "Roaming")
		}
		base = filepath.Join(roaming, "Mozilla", "Firefox", "Profiles")
	default:
		base = filepath.Join(home, ".mozilla", "firefox")
	}

	entries, err := os.ReadDir(base)
	if err != nil {
		return "", fmt.Errorf("read firefox profiles dir: %w", err)
	}
	for _, e := range entries {
		if e.IsDir() && strings.HasSuffix(e.Name(), ".default-release") {
			return filepath.Join(base, e.Name()), nil
		}
	}
	for _, e := range entries {
		if e.IsDir() && strings.HasSuffix(e.Name(), ".default") {
			return filepath.Join(base, e.Name()), nil
		}
	}
	return "", fmt.Errorf("no firefox profile found in %s", base)
}
