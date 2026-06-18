//go:build manual

package browserid

import (
	"testing"
)

func TestExtractInstalledBrowsers(t *testing.T) {
	for _, b := range []string{"chrome", "chromium", "firefox", "edge", "brave"} {
		res, err := Extract(b)
		if err != nil {
			t.Logf("%s: %v", b, err)
			continue
		}
		t.Logf("%s: found %s (source: %s)", b, res.ClientID, res.Source)
	}
}
