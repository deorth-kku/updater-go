package downloader

import (
	"testing"
)

func TestErrNotLocal(t *testing.T) {
	if ErrNotLocal == nil {
		t.Error("ErrNotLocal should not be nil")
	}
	if ErrNotLocal.Error() != "not a local address" {
		t.Errorf("ErrNotLocal.Error() = %q, want %q", ErrNotLocal.Error(), "not a local address")
	}
}

func TestGenerateSecret_Length(t *testing.T) {
	for i := 0; i < 10; i++ {
		secret := generateSecret()
		if len(secret) != 16 {
			t.Errorf("generateSecret() len = %d, want 16 (iteration %d)", len(secret), i)
		}
	}
}

func TestGenerateSecret_AllHex(t *testing.T) {
	secret := generateSecret()
	if len(secret) != 16 {
		t.Fatalf("generateSecret() len = %d, want 16", len(secret))
	}
	for i, c := range secret {
		valid := (c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')
		if !valid {
			t.Errorf("generateSecret()[%d] = %c, not a valid hex char", i, c)
		}
	}
}

func TestGenerateSecret_Lowercase(t *testing.T) {
	secret := generateSecret()
	if len(secret) != 16 {
		t.Fatalf("generateSecret() len = %d, want 16", len(secret))
	}
	for i, c := range secret {
		if c >= 'A' && c <= 'Z' {
			t.Errorf("generateSecret()[%d] = %c, should be lowercase", i, c)
		}
	}
}
