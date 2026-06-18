//go:build manual

package auth

import (
	"testing"
)

func TestValidateClientID(t *testing.T) {
	// This is the public Spotify web player Client ID extracted from Chromium
	// Local Storage. It is used here only to verify the validation probe.
	const candidate = "d8a5ed958d274c2e8ee717e6a4b0971d"
	if err := ValidateClientID(candidate); err != nil {
		t.Logf("validate %q: %v", candidate, err)
	} else {
		t.Logf("validate %q: ok", candidate)
	}
}
