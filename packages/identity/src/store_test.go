package identity

import (
	"testing"
	"time"
)

func TestIdentityStore(t *testing.T) {
	home := t.TempDir()
	store, err := Open(home)
	if err != nil {
		t.Fatal(err)
	}

	tok, err := store.CreateToken("user_123", "user@example.com", 1*time.Hour)
	if err != nil {
		t.Fatal(err)
	}

	val, err := store.Validate(tok.ID)
	if err != nil || val.UserID != "user_123" {
		t.Fatalf("validation failed: err=%v, val=%#v", err, val)
	}

	if err := store.Revoke(tok.ID); err != nil {
		t.Fatal(err)
	}

	if _, err := store.Validate(tok.ID); err == nil {
		t.Fatal("expected error validating revoked token")
	}
}
