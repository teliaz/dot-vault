package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

const configVersion = 1

type Manager struct {
	configPath string
}

type Config struct {
	Version            int                     `json:"version"`
	ActiveOrganization string                  `json:"active_organization,omitempty"`
	Organizations      map[string]Organization `json:"organizations"`
}

type Organization struct {
	Name             string     `json:"name"`
	RepoRoot         string     `json:"repo_root"`
	StoreRoot        string     `json:"store_root"`
	MasterKeyBackend string     `json:"master_key_backend"`
	AuthPolicy       AuthPolicy `json:"auth_policy"`
	CreatedAt        time.Time  `json:"created_at"`
	UpdatedAt        time.Time  `json:"updated_at"`
}

type AuthPolicy struct {
	SessionTTLMinutes int      `json:"session_ttl_minutes"`
	SensitiveActions  []string `json:"sensitive_actions"`
}

func NewManager() *Manager {
	return &Manager{
		configPath: defaultConfigPath(),
	}
}

func NewManagerWithPath(configPath string) *Manager {
	return &Manager{configPath: configPath}
}

func (m *Manager) Load() (*Config, error) {
	contents, err := os.ReadFile(m.configPath)
	if errors.Is(err, os.ErrNotExist) {
		return &Config{
			Version:       configVersion,
			Organizations: map[string]Organization{},
		}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}

	var cfg Config
	if err := json.Unmarshal(contents, &cfg); err != nil {
		return nil, fmt.Errorf("decode config: %w", err)
	}

	if cfg.Version == 0 {
		cfg.Version = configVersion
	}
	if cfg.Organizations == nil {
		cfg.Organizations = map[string]Organization{}
	}

	return &cfg, nil
}

func (m *Manager) Save(cfg *Config) error {
	if cfg.Version == 0 {
		cfg.Version = configVersion
	}
	if cfg.Organizations == nil {
		cfg.Organizations = map[string]Organization{}
	}

	if err := os.MkdirAll(filepath.Dir(m.configPath), 0o700); err != nil {
		return fmt.Errorf("create config directory: %w", err)
	}

	payload, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("encode config: %w", err)
	}
	payload = append(payload, '\n')

	if err := os.WriteFile(m.configPath, payload, 0o600); err != nil {
		return fmt.Errorf("write config: %w", err)
	}
	return nil
}

func (m *Manager) ConfigPath() string {
	return m.configPath
}

func defaultConfigPath() string {
	if override := os.Getenv("DOT_VAULT_CONFIG"); strings.TrimSpace(override) != "" {
		return override
	}

	baseDir, err := os.UserConfigDir()
	if err != nil {
		home, homeErr := os.UserHomeDir()
		if homeErr != nil {
			return "dot-vault-config.json"
		}
		baseDir = filepath.Join(home, ".config")
	}

	appDir := "dot-vault"
	if runtime.GOOS == "darwin" {
		appDir = "DotVault"
	}

	return filepath.Join(baseDir, appDir, "config.json")
}
