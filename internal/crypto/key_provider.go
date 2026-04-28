package crypto

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"strings"

	keyring "github.com/zalando/go-keyring"
)

const (
	defaultServiceName = "dot-vault"
	masterKeySize      = 32
)

type KeyProvider struct {
	serviceName      string
	fallbackProvider *PassphraseProvider
}

func NewKeyProvider(serviceName string) *KeyProvider {
	normalized := strings.TrimSpace(serviceName)
	if normalized == "" {
		normalized = defaultServiceName
	}
	return &KeyProvider{
		serviceName:      normalized,
		fallbackProvider: NewPassphraseProvider(os.Stdin, os.Stderr),
	}
}

func (p *KeyProvider) GetOrCreateMasterKey(ctx context.Context, organization string) ([]byte, error) {
	if strings.TrimSpace(organization) == "" {
		return nil, fmt.Errorf("organization is required")
	}
	if p.fallbackProvider.HasPassphrase(organization) {
		return p.fallbackProvider.GetOrCreateMasterKey(ctx, organization)
	}

	existing, err := keyring.Get(p.serviceName, organization)
	if err == nil {
		key, decodeErr := base64.StdEncoding.DecodeString(existing)
		if decodeErr != nil {
			return nil, fmt.Errorf("decode stored master key: %w", decodeErr)
		}
		if len(key) != masterKeySize {
			return nil, fmt.Errorf("stored master key has invalid length %d", len(key))
		}
		return key, nil
	}

	if !errors.Is(err, keyring.ErrNotFound) {
		return p.fallbackMasterKey(ctx, organization, fmt.Errorf("read master key from keyring: %w", err))
	}

	key := make([]byte, masterKeySize)
	if _, err := rand.Read(key); err != nil {
		return nil, fmt.Errorf("generate master key: %w", err)
	}

	encoded := base64.StdEncoding.EncodeToString(key)
	if err := keyring.Set(p.serviceName, organization, encoded); err != nil {
		return p.fallbackMasterKey(ctx, organization, fmt.Errorf("store master key in keyring: %w", err))
	}

	return key, nil
}

func (p *KeyProvider) SetPassphrase(organization string, passphrase string) error {
	return p.fallbackProvider.SetPassphrase(organization, passphrase)
}

func (p *KeyProvider) SetInteractiveFallback(interactive bool) {
	p.fallbackProvider.SetInteractive(interactive)
}

func (p *KeyProvider) fallbackMasterKey(ctx context.Context, organization string, keyringErr error) ([]byte, error) {
	if p.fallbackProvider == nil {
		return nil, keyringErr
	}
	key, err := p.fallbackProvider.GetOrCreateMasterKey(ctx, organization)
	if err != nil {
		return nil, fmt.Errorf("%w; passphrase fallback failed: %w", keyringErr, err)
	}
	return key, nil
}
