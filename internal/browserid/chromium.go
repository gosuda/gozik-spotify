package browserid

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/syndtr/goleveldb/leveldb"
)

// extractChromium searches Chromium-family Local Storage LevelDB for a Spotify Client ID.
func extractChromium(browser string) (Result, error) {
	dir := chromiumLocalStorageDir(browser)
	if _, err := os.Stat(dir); err != nil {
		return Result{}, fmt.Errorf("%s local storage not found: %w", browser, err)
	}

	db, err := leveldb.OpenFile(dir, nil)
	if err != nil {
		return Result{}, fmt.Errorf("open %s leveldb: %w (try closing the browser first)", browser, err)
	}
	defer db.Close()

	var candidates []Result
	iter := db.NewIterator(nil, nil)
	defer iter.Release()

	for iter.Next() {
		key := string(iter.Key())
		value := string(iter.Value())
		if !isSpotifyKey(key) {
			continue
		}
		context := fmt.Sprintf("%s:localStorage:%s:%s", browser, key, value)
		for _, m := range clientIDRe.FindAllString(value, -1) {
			candidates = append(candidates, Result{
				ClientID: normalizeClientID(m),
				Source:   context,
			})
		}
		// Some sites store the client id in the key itself.
		for _, m := range clientIDRe.FindAllString(key, -1) {
			candidates = append(candidates, Result{
				ClientID: normalizeClientID(m),
				Source:   context,
			})
		}
	}
	if err := iter.Error(); err != nil {
		return Result{}, fmt.Errorf("iterate %s leveldb: %w", browser, err)
	}

	// Also scan the raw LevelDB files as a fallback for entries that may not
	// survive the iterator (e.g. stale/deleted records still on disk).
	if len(candidates) == 0 {
		if raw, err := scanRawLevelDB(dir, browser); err == nil {
			candidates = append(candidates, raw...)
		}
	}

	best, err := pickBest(candidates)
	if err != nil {
		return Result{}, fmt.Errorf("%s: %w", browser, err)
	}
	return best, nil
}

// isSpotifyKey guesses whether a LevelDB key belongs to a Spotify origin.
func isSpotifyKey(key string) bool {
	low := strings.ToLower(key)
	return strings.Contains(low, "spotify") ||
		strings.Contains(low, "developer.spotify") ||
		strings.Contains(low, "open.spotify") ||
		strings.Contains(low, "accounts.spotify")
}

// scanRawLevelDB reads the raw .ldb/.log files as a fallback. It is tolerant
// of the LevelDB binary format and just looks for printable context around
// candidate hex IDs.
func scanRawLevelDB(dir, browser string) ([]Result, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var candidates []Result
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		ext := filepath.Ext(e.Name())
		if ext != ".ldb" && ext != ".log" && ext != ".sst" {
			continue
		}
		path := filepath.Join(dir, e.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		text := string(data)
		context := fmt.Sprintf("%s:leveldb_raw:%s", browser, e.Name())
		if !strings.Contains(strings.ToLower(text), "spotify") {
			continue
		}
		for _, m := range clientIDRe.FindAllString(text, -1) {
			candidates = append(candidates, Result{
				ClientID: normalizeClientID(m),
				Source:   context,
			})
		}
	}
	return candidates, nil
}
