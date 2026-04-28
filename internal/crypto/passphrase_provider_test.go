package crypto

import (
	"context"
	"testing"
)

func TestDerivePassphraseMasterKeyIsStable(t *testing.T) {
	t.Parallel()

	first := derivePassphraseMasterKey("acme", "correct horse battery staple")
	second := derivePassphraseMasterKey("acme", "correct horse battery staple")

	if len(first) != masterKeySize {
		t.Fatalf("key length = %d, want %d", len(first), masterKeySize)
	}
	if string(first) != string(second) {
		t.Fatalf("derived keys differ for same organization and passphrase")
	}
}

func TestDerivePassphraseMasterKeyIsOrganizationScoped(t *testing.T) {
	t.Parallel()

	first := derivePassphraseMasterKey("acme", "correct horse battery staple")
	second := derivePassphraseMasterKey("other", "correct horse battery staple")

	if string(first) == string(second) {
		t.Fatalf("derived keys matched across organizations")
	}
}

func TestPassphraseProviderRequiresMinimumLength(t *testing.T) {
	t.Setenv(passphraseEnvVar, "too-short")

	_, err := NewPassphraseProvider(nil, nil).GetOrCreateMasterKey(context.Background(), "acme")
	if err == nil {
		t.Fatalf("GetOrCreateMasterKey() error = nil, want error")
	}
}

func TestPassphraseProviderUsesEnvironmentPassphrase(t *testing.T) {
	t.Setenv(passphraseEnvVar, "correct horse battery staple")

	key, err := NewPassphraseProvider(nil, nil).GetOrCreateMasterKey(context.Background(), "acme")
	if err != nil {
		t.Fatalf("GetOrCreateMasterKey() error = %v", err)
	}
	if len(key) != masterKeySize {
		t.Fatalf("key length = %d, want %d", len(key), masterKeySize)
	}
}

func TestKeyProviderUsesExplicitPassphraseBeforeKeyring(t *testing.T) {
	t.Parallel()

	provider := NewKeyProvider("dot-vault-test")
	if err := provider.SetPassphrase("acme", "correct horse battery staple"); err != nil {
		t.Fatalf("SetPassphrase() error = %v", err)
	}

	key, err := provider.GetOrCreateMasterKey(context.Background(), "acme")
	if err != nil {
		t.Fatalf("GetOrCreateMasterKey() error = %v", err)
	}
	want := derivePassphraseMasterKey("acme", "correct horse battery staple")
	if string(key) != string(want) {
		t.Fatalf("key did not use explicit passphrase")
	}
}

func TestPassphraseProviderNonInteractiveReturnsError(t *testing.T) {
	t.Parallel()

	provider := NewPassphraseProvider(nil, nil)
	provider.SetInteractive(false)

	_, err := provider.GetOrCreateMasterKey(context.Background(), "acme")
	if err == nil {
		t.Fatalf("GetOrCreateMasterKey() error = nil, want error")
	}
}
