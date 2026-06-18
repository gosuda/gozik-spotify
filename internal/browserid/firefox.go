package browserid

import (
	"database/sql"
	"fmt"
	"strings"

	_ "modernc.org/sqlite"
)

// extractFirefox searches Firefox webappsstore.sqlite for a Spotify Client ID.
func extractFirefox() (Result, error) {
	profile, err := firefoxProfileDir()
	if err != nil {
		return Result{}, err
	}

	dbPath := fmt.Sprintf("file:%s/webappsstore.sqlite?_pragma=busy_timeout(5000)&mode=ro", profile)
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return Result{}, fmt.Errorf("open firefox storage: %w", err)
	}
	defer db.Close()

	var candidates []Result
	// webappsstore2 is the modern table name; fall back to webappsstore.
	for _, table := range []string{"webappsstore2", "webappsstore"} {
		rows, err := db.Query(fmt.Sprintf(
			"SELECT scope, key, value FROM %s WHERE value LIKE '%%spotify%%' OR scope LIKE '%%spotify%%' OR key LIKE '%%spotify%%'",
			table))
		if err != nil {
			continue
		}
		defer rows.Close()
		for rows.Next() {
			var scope, key, value string
			if err := rows.Scan(&scope, &key, &value); err != nil {
				continue
			}
			context := fmt.Sprintf("firefox:webappsstore:%s:%s:%s", scope, key, value)
			for _, m := range clientIDRe.FindAllString(value, -1) {
				candidates = append(candidates, Result{
					ClientID: normalizeClientID(m),
					Source:   context,
				})
			}
			for _, m := range clientIDRe.FindAllString(key, -1) {
				candidates = append(candidates, Result{
					ClientID: normalizeClientID(m),
					Source:   context,
				})
			}
		}
		_ = rows.Close()
	}

	best, err := pickBest(candidates)
	if err != nil {
		return Result{}, fmt.Errorf("firefox: %w", err)
	}
	return best, nil
}

// isSpotifyScope guesses whether a Firefox storage scope/key belongs to Spotify.
func isSpotifyScope(scope, key string) bool {
	for _, s := range []string{scope, key} {
		low := strings.ToLower(s)
		if strings.Contains(low, "spotify") {
			return true
		}
	}
	return false
}
