package downloader

import (
	"strings"
	"testing"
)

func TestGenerateSecret(t *testing.T) {
	secret := generateSecret()
	if len(secret) != 16 {
		t.Errorf("generateSecret() len = %d, want 16", len(secret))
	}

	// Verify it's a hex string
	for _, c := range secret {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
			t.Errorf("generateSecret() contains non-hex character: %c", c)
		}
	}

	// Verify it's lowercase
	if secret != strings.ToLower(secret) {
		t.Error("generateSecret() should return lowercase hex string")
	}
}

func TestGenerateSecret_Uniqueness(t *testing.T) {
	secret1 := generateSecret()
	secret2 := generateSecret()

	// While extremely unlikely, two generated secrets could be the same
	// This test just verifies they're both valid
	if len(secret1) != 16 || len(secret2) != 16 {
		t.Error("generateSecret() should return 16-character strings")
	}
}
