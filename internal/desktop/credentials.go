package desktop

import (
	"errors"
	"fmt"

	"github.com/egoist/mygo"
	"github.com/wxxb789/passquest/internal/cli"
)

// secrets is the part of mygo.SecretsModule the credential store uses.
type secrets interface {
	Get(key string) ([]byte, error)
	Set(key string, value []byte) error
	Delete(key string) error
}

// credentialAccount is the account a provider's API key is stored under, in
// the service named by secretService.
func credentialAccount(provider string) string { return "api-key:" + provider }

// secretStore is the workspace's CredentialStore over the OS credential store
// (Q-0011-03). Errors never carry the key.
type secretStore struct{ secrets secrets }

func (s secretStore) Get(provider string) (string, bool, error) {
	value, err := s.secrets.Get(credentialAccount(provider))
	switch {
	case errors.Is(err, mygo.ErrSecretNotFound):
		return "", false, nil
	case err != nil:
		return "", false, fmt.Errorf("read credential for %s: %w", provider, err)
	}
	return string(value), true, nil
}

func (s secretStore) Set(provider, key string) error {
	if err := s.secrets.Set(credentialAccount(provider), []byte(key)); err != nil {
		return fmt.Errorf("store credential for %s: %w", provider, err)
	}
	return nil
}

func (s secretStore) Delete(provider string) error {
	if err := s.secrets.Delete(credentialAccount(provider)); err != nil {
		return fmt.Errorf("delete credential for %s: %w", provider, err)
	}
	return nil
}

// newCredentialStore returns the store over secrets, or nil when this system
// has no credential store (errors.ErrUnsupported, e.g. Linux without a Secret
// Service): the workspace then reports credential_store_unavailable and the
// environment variable stays the only path.
func newCredentialStore(secrets secrets) cli.CredentialStore {
	if _, err := secrets.Get(credentialAccount("probe")); errors.Is(err, errors.ErrUnsupported) {
		return nil
	}
	return secretStore{secrets: secrets}
}
