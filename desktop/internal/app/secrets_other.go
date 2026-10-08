//go:build !windows

package app

type unavailableSecretStore struct{}

func DefaultSecretStore() SecretStore                      { return unavailableSecretStore{} }
func (unavailableSecretStore) Status(string) (bool, error) { return false, errCredentialUnavailable }
func (unavailableSecretStore) Read(string) (string, error) { return "", errCredentialUnavailable }
func (unavailableSecretStore) Write(string, string) error  { return errCredentialUnavailable }
func (unavailableSecretStore) Delete(string) error         { return errCredentialUnavailable }
