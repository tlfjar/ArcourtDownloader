//go:build windows

package app

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"testing"
)

func TestWindowsCredentialManagerRoundtrip(t *testing.T) {
	var id [12]byte
	if _, err := rand.Read(id[:]); err != nil {
		t.Fatal(err)
	}
	store := credentialManager{prefix: "ArcourtDownloader/Test/" + hex.EncodeToString(id[:]) + "/"}
	const provider, key = "openai", "synthetic-test-key"
	t.Cleanup(func() { _ = store.Delete(provider) })
	if _, err := store.Read(provider); !errors.Is(err, ErrCredentialMissing) {
		t.Fatalf("missing credential: %v", err)
	}
	if configured, err := store.Status(provider); err != nil || configured {
		t.Fatalf("missing status: %t, %v", configured, err)
	}
	if err := store.Write(provider, key); err != nil {
		t.Fatalf("write: %v", err)
	}
	if got, err := store.Read(provider); err != nil || got != key {
		t.Fatalf("read: %q, %v", got, err)
	}
	if configured, err := store.Status(provider); err != nil || !configured {
		t.Fatalf("configured status: %t, %v", configured, err)
	}
	if err := store.Write(provider, "replacement-synthetic-key"); err != nil {
		t.Fatalf("replace: %v", err)
	}
	if got, err := store.Read(provider); err != nil || got != "replacement-synthetic-key" {
		t.Fatalf("replacement: %q, %v", got, err)
	}
	if err := store.Delete(provider); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, err := store.Read(provider); !errors.Is(err, ErrCredentialMissing) {
		t.Fatalf("removed credential: %v", err)
	}
}
