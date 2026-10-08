package app

import (
	"errors"
	"strings"
)

var ErrCredentialMissing = errors.New("naming credential is missing")
var errCredentialUnavailable = errors.New("secure credential storage is unavailable")

// SecretStore is deliberately narrow. Stored credentials never enter snapshots,
// settings JSON, or the browser-backed frontend after a write completes.
type SecretStore interface {
	Status(provider string) (bool, error)
	Read(provider string) (string, error)
	Write(provider, key string) error
	Delete(provider string) error
}

func validateCredential(provider, key string) (string, error) {
	if namingRecipient(provider) == "" {
		return "", errors.New("Choose a supported provider before saving a credential.")
	}
	key = strings.TrimSpace(key)
	if key == "" || len(key) > 2048 || strings.IndexFunc(key, func(r rune) bool { return r < 0x20 || r == 0x7f }) >= 0 {
		return "", errors.New("Enter a provider API key of at most 2048 characters without control characters.")
	}
	return key, nil
}
