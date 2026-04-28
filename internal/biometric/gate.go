package biometric

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/teliaz/dot-vault/internal/config"
)

type masterKeyProvider interface {
	GetOrCreateMasterKey(ctx context.Context, organization string) ([]byte, error)
}

type sensitiveAuthorizer interface {
	Authorize(ctx context.Context, org config.Organization, action string) error
}

type Gate struct {
	keyProvider masterKeyProvider
	authorizer  sensitiveAuthorizer
	sessionPath string
	now         func() time.Time
}

type sessionFile struct {
	Organizations map[string]session `json:"organizations"`
}

type session struct {
	AuthorizedUntil time.Time `json:"authorized_until"`
}

func NewGate(keyProvider masterKeyProvider) *Gate {
	return &Gate{
		keyProvider: keyProvider,
		authorizer:  newPlatformAuthorizer(),
		sessionPath: defaultSessionPath(),
		now:         func() time.Time { return time.Now().UTC() },
	}
}

func NewGateWithSessionPath(keyProvider masterKeyProvider, sessionPath string, now func() time.Time) *Gate {
	if now == nil {
		now = func() time.Time { return time.Now().UTC() }
	}
	return &Gate{
		keyProvider: keyProvider,
		sessionPath: sessionPath,
		now:         now,
	}
}

func (g *Gate) Authorize(ctx context.Context, org config.Organization, action string) error {
	action = strings.TrimSpace(action)
	if action == "" {
		return fmt.Errorf("sensitive action is required")
	}
	if !requiresAuthorization(org.AuthPolicy, action) {
		return nil
	}
	if org.AuthPolicy.SessionTTLMinutes <= 0 {
		return g.challenge(ctx, org, action)
	}

	sessions, err := g.loadSessions()
	if err != nil {
		return err
	}

	now := g.now()
	if existing, ok := sessions.Organizations[org.Name]; ok && existing.AuthorizedUntil.After(now) {
		return nil
	}

	if err := g.challenge(ctx, org, action); err != nil {
		return err
	}

	sessions.Organizations[org.Name] = session{
		AuthorizedUntil: now.Add(time.Duration(org.AuthPolicy.SessionTTLMinutes) * time.Minute),
	}
	return g.saveSessions(sessions)
}

func (g *Gate) challenge(ctx context.Context, org config.Organization, action string) error {
	if strings.TrimSpace(org.Name) == "" {
		return fmt.Errorf("organization name is required")
	}
	if g.authorizer != nil {
		err := g.authorizer.Authorize(ctx, org, action)
		if err == nil {
			return nil
		}
		if !errors.Is(err, ErrTouchIDUnavailable) {
			return err
		}
	}
	if g.keyProvider == nil {
		return fmt.Errorf("no authorization backend is available")
	}
	if _, err := g.keyProvider.GetOrCreateMasterKey(ctx, org.Name); err != nil {
		return fmt.Errorf("authorize %s for %s through %s: %w", action, org.Name, org.MasterKeyBackend, err)
	}
	return nil
}

func (g *Gate) loadSessions() (sessionFile, error) {
	payload, err := os.ReadFile(g.sessionPath)
	if errors.Is(err, os.ErrNotExist) {
		return sessionFile{Organizations: map[string]session{}}, nil
	}
	if err != nil {
		return sessionFile{}, fmt.Errorf("read authorization session: %w", err)
	}

	var sessions sessionFile
	if err := json.Unmarshal(payload, &sessions); err != nil {
		return sessionFile{}, fmt.Errorf("decode authorization session: %w", err)
	}
	if sessions.Organizations == nil {
		sessions.Organizations = map[string]session{}
	}
	return sessions, nil
}

func (g *Gate) saveSessions(sessions sessionFile) error {
	if sessions.Organizations == nil {
		sessions.Organizations = map[string]session{}
	}
	if err := os.MkdirAll(filepath.Dir(g.sessionPath), 0o700); err != nil {
		return fmt.Errorf("create authorization session directory: %w", err)
	}

	payload, err := json.MarshalIndent(sessions, "", "  ")
	if err != nil {
		return fmt.Errorf("encode authorization session: %w", err)
	}
	payload = append(payload, '\n')
	if err := os.WriteFile(g.sessionPath, payload, 0o600); err != nil {
		return fmt.Errorf("write authorization session: %w", err)
	}
	return nil
}

func requiresAuthorization(policy config.AuthPolicy, action string) bool {
	sensitiveActions := policy.SensitiveActions
	if len(sensitiveActions) == 0 {
		sensitiveActions = []string{"reveal", "restore", "backup"}
	}

	for _, sensitiveAction := range sensitiveActions {
		if sensitiveAction == action {
			return true
		}
	}
	return false
}

func defaultSessionPath() string {
	if override := strings.TrimSpace(os.Getenv("DOT_VAULT_SESSION_FILE")); override != "" {
		return override
	}

	baseDir, err := os.UserCacheDir()
	if err != nil {
		home, homeErr := os.UserHomeDir()
		if homeErr != nil {
			return "dot-vault-session.json"
		}
		baseDir = filepath.Join(home, ".cache")
	}
	return filepath.Join(baseDir, "dot-vault", "session.json")
}
