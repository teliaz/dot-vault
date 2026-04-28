package orgs

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/teliaz/dot-vault/internal/config"
)

type Service struct {
	configManager *config.Manager
}

func NewService(configManager *config.Manager) *Service {
	return &Service{configManager: configManager}
}

func (s *Service) Add(_ context.Context, name string, repoRoot string, storeRoot string, setActive bool) (config.Organization, error) {
	cfg, err := s.configManager.Load()
	if err != nil {
		return config.Organization{}, err
	}

	normalizedName := strings.TrimSpace(name)
	if normalizedName == "" {
		return config.Organization{}, fmt.Errorf("organization name is required")
	}
	if _, exists := cfg.Organizations[normalizedName]; exists {
		return config.Organization{}, fmt.Errorf("organization %q already exists", normalizedName)
	}

	absRepoRoot, err := expandAndAbsPath(repoRoot)
	if err != nil {
		return config.Organization{}, fmt.Errorf("resolve repo root: %w", err)
	}
	repoInfo, err := os.Stat(absRepoRoot)
	if err != nil {
		return config.Organization{}, fmt.Errorf("stat repo root: %w", err)
	}
	if !repoInfo.IsDir() {
		return config.Organization{}, fmt.Errorf("repo root must be a directory")
	}

	absStoreRoot, err := expandAndAbsPath(storeRoot)
	if err != nil {
		return config.Organization{}, fmt.Errorf("resolve store root: %w", err)
	}
	if err := os.MkdirAll(absStoreRoot, 0o700); err != nil {
		return config.Organization{}, fmt.Errorf("create store root: %w", err)
	}

	now := time.Now().UTC()
	org := config.Organization{
		Name:             normalizedName,
		RepoRoot:         absRepoRoot,
		StoreRoot:        absStoreRoot,
		MasterKeyBackend: defaultMasterKeyBackend(),
		AuthPolicy: config.AuthPolicy{
			SessionTTLMinutes: 15,
			SensitiveActions: []string{
				"reveal",
				"restore",
				"backup",
			},
		},
		CreatedAt: now,
		UpdatedAt: now,
	}

	cfg.Organizations[org.Name] = org
	if setActive || cfg.ActiveOrganization == "" {
		cfg.ActiveOrganization = org.Name
	}

	if err := s.configManager.Save(cfg); err != nil {
		return config.Organization{}, err
	}

	return org, nil
}

func (s *Service) SetActive(_ context.Context, name string) (config.Organization, error) {
	cfg, err := s.configManager.Load()
	if err != nil {
		return config.Organization{}, err
	}

	normalizedName := strings.TrimSpace(name)
	if normalizedName == "" {
		return config.Organization{}, fmt.Errorf("organization name is required")
	}

	org, ok := cfg.Organizations[normalizedName]
	if !ok {
		return config.Organization{}, fmt.Errorf("organization %q not found", normalizedName)
	}

	cfg.ActiveOrganization = normalizedName
	org.UpdatedAt = time.Now().UTC()
	cfg.Organizations[normalizedName] = org

	if err := s.configManager.Save(cfg); err != nil {
		return config.Organization{}, err
	}
	return org, nil
}

func (s *Service) Remove(_ context.Context, name string) (config.Organization, error) {
	cfg, err := s.configManager.Load()
	if err != nil {
		return config.Organization{}, err
	}

	normalizedName := strings.TrimSpace(name)
	if normalizedName == "" {
		return config.Organization{}, fmt.Errorf("organization name is required")
	}

	removed, ok := cfg.Organizations[normalizedName]
	if !ok {
		return config.Organization{}, fmt.Errorf("organization %q not found", normalizedName)
	}
	delete(cfg.Organizations, normalizedName)

	if cfg.ActiveOrganization == normalizedName {
		cfg.ActiveOrganization = ""
		for candidate := range cfg.Organizations {
			if cfg.ActiveOrganization == "" || candidate < cfg.ActiveOrganization {
				cfg.ActiveOrganization = candidate
			}
		}
	}

	if err := s.configManager.Save(cfg); err != nil {
		return config.Organization{}, err
	}
	return removed, nil
}

func defaultMasterKeyBackend() string {
	if runtime.GOOS == "darwin" {
		return "keychain"
	}
	return "keyring"
}

func expandAndAbsPath(path string) (string, error) {
	trimmed := strings.TrimSpace(path)
	if trimmed == "" {
		return "", fmt.Errorf("path is required")
	}
	if trimmed == "~" || strings.HasPrefix(trimmed, "~/") {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("resolve home directory: %w", err)
		}
		if trimmed == "~" {
			trimmed = homeDir
		} else {
			trimmed = filepath.Join(homeDir, strings.TrimPrefix(trimmed, "~/"))
		}
	}
	return filepath.Abs(trimmed)
}
