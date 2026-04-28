package biometric

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/teliaz/dot-vault/internal/config"
)

type fakeMasterKeyProvider struct {
	calls int
}

type fakeAuthorizer struct {
	calls int
	err   error
}

func (f *fakeMasterKeyProvider) GetOrCreateMasterKey(_ context.Context, _ string) ([]byte, error) {
	f.calls++
	return []byte("0123456789abcdef0123456789abcdef"), nil
}

func (f *fakeAuthorizer) Authorize(_ context.Context, _ config.Organization, _ string) error {
	f.calls++
	return f.err
}

func TestAuthorizeCachesSessionForTTL(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 4, 28, 12, 0, 0, 0, time.UTC)
	provider := &fakeMasterKeyProvider{}
	gate := NewGateWithSessionPath(provider, filepath.Join(t.TempDir(), "session.json"), func() time.Time {
		return now
	})
	org := config.Organization{
		Name:             "acme",
		MasterKeyBackend: "keyring",
		AuthPolicy: config.AuthPolicy{
			SessionTTLMinutes: 15,
			SensitiveActions:  []string{"reveal", "restore", "backup"},
		},
	}

	if err := gate.Authorize(context.Background(), org, "restore"); err != nil {
		t.Fatalf("Authorize() error = %v", err)
	}
	if err := gate.Authorize(context.Background(), org, "backup"); err != nil {
		t.Fatalf("Authorize() cached error = %v", err)
	}
	if provider.calls != 1 {
		t.Fatalf("provider calls = %d, want 1", provider.calls)
	}
}

func TestAuthorizeUsesPlatformAuthorizerBeforeKeyProvider(t *testing.T) {
	t.Parallel()

	provider := &fakeMasterKeyProvider{}
	authorizer := &fakeAuthorizer{}
	gate := NewGateWithSessionPath(provider, filepath.Join(t.TempDir(), "session.json"), nil)
	gate.authorizer = authorizer
	org := config.Organization{
		Name: "acme",
		AuthPolicy: config.AuthPolicy{
			SensitiveActions: []string{"restore"},
		},
	}

	if err := gate.Authorize(context.Background(), org, "restore"); err != nil {
		t.Fatalf("Authorize() error = %v", err)
	}
	if authorizer.calls != 1 {
		t.Fatalf("authorizer calls = %d, want 1", authorizer.calls)
	}
	if provider.calls != 0 {
		t.Fatalf("provider calls = %d, want 0", provider.calls)
	}
}

func TestAuthorizeFallsBackWhenTouchIDUnavailable(t *testing.T) {
	t.Parallel()

	provider := &fakeMasterKeyProvider{}
	authorizer := &fakeAuthorizer{err: ErrTouchIDUnavailable}
	gate := NewGateWithSessionPath(provider, filepath.Join(t.TempDir(), "session.json"), nil)
	gate.authorizer = authorizer
	org := config.Organization{
		Name:             "acme",
		MasterKeyBackend: "keyring",
		AuthPolicy: config.AuthPolicy{
			SensitiveActions: []string{"backup"},
		},
	}

	if err := gate.Authorize(context.Background(), org, "backup"); err != nil {
		t.Fatalf("Authorize() error = %v", err)
	}
	if authorizer.calls != 1 {
		t.Fatalf("authorizer calls = %d, want 1", authorizer.calls)
	}
	if provider.calls != 1 {
		t.Fatalf("provider calls = %d, want 1", provider.calls)
	}
}

func TestAuthorizeDoesNotFallBackWhenTouchIDDenied(t *testing.T) {
	t.Parallel()

	provider := &fakeMasterKeyProvider{}
	authorizer := &fakeAuthorizer{err: fmt.Errorf("touch id authorization failed")}
	gate := NewGateWithSessionPath(provider, filepath.Join(t.TempDir(), "session.json"), nil)
	gate.authorizer = authorizer
	org := config.Organization{
		Name: "acme",
		AuthPolicy: config.AuthPolicy{
			SensitiveActions: []string{"reveal"},
		},
	}

	if err := gate.Authorize(context.Background(), org, "reveal"); err == nil {
		t.Fatalf("Authorize() error = nil, want error")
	}
	if provider.calls != 0 {
		t.Fatalf("provider calls = %d, want 0", provider.calls)
	}
}

func TestAuthorizeChallengesAfterSessionExpiry(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 4, 28, 12, 0, 0, 0, time.UTC)
	provider := &fakeMasterKeyProvider{}
	gate := NewGateWithSessionPath(provider, filepath.Join(t.TempDir(), "session.json"), func() time.Time {
		return now
	})
	org := config.Organization{
		Name:             "acme",
		MasterKeyBackend: "keyring",
		AuthPolicy: config.AuthPolicy{
			SessionTTLMinutes: 15,
			SensitiveActions:  []string{"restore"},
		},
	}

	if err := gate.Authorize(context.Background(), org, "restore"); err != nil {
		t.Fatalf("Authorize() error = %v", err)
	}
	now = now.Add(16 * time.Minute)
	if err := gate.Authorize(context.Background(), org, "restore"); err != nil {
		t.Fatalf("Authorize() after expiry error = %v", err)
	}
	if provider.calls != 2 {
		t.Fatalf("provider calls = %d, want 2", provider.calls)
	}
}

func TestAuthorizeSkipsNonSensitiveAction(t *testing.T) {
	t.Parallel()

	provider := &fakeMasterKeyProvider{}
	gate := NewGateWithSessionPath(provider, filepath.Join(t.TempDir(), "session.json"), nil)
	org := config.Organization{
		Name: "acme",
		AuthPolicy: config.AuthPolicy{
			SessionTTLMinutes: 15,
			SensitiveActions:  []string{"restore"},
		},
	}

	if err := gate.Authorize(context.Background(), org, "import"); err != nil {
		t.Fatalf("Authorize() error = %v", err)
	}
	if provider.calls != 0 {
		t.Fatalf("provider calls = %d, want 0", provider.calls)
	}
}

func TestAuthorizeUsesDefaultSensitiveActions(t *testing.T) {
	t.Parallel()

	provider := &fakeMasterKeyProvider{}
	gate := NewGateWithSessionPath(provider, filepath.Join(t.TempDir(), "session.json"), nil)
	org := config.Organization{Name: "acme"}

	if err := gate.Authorize(context.Background(), org, "reveal"); err != nil {
		t.Fatalf("Authorize() error = %v", err)
	}
	if provider.calls != 1 {
		t.Fatalf("provider calls = %d, want 1", provider.calls)
	}
}
