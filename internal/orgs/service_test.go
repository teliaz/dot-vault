package orgs

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/teliaz/dot-vault/internal/config"
)

func TestAddOrganizationSetsDefaultsAndPersistsConfig(t *testing.T) {
	t.Parallel()

	tempDir := t.TempDir()
	repoRoot := filepath.Join(tempDir, "repos")
	storeRoot := filepath.Join(tempDir, "store")
	configPath := filepath.Join(tempDir, "config.json")

	if err := os.MkdirAll(repoRoot, 0o755); err != nil {
		t.Fatalf("mkdir repo root: %v", err)
	}

	manager := config.NewManagerWithPath(configPath)
	service := NewService(manager)

	org, err := service.Add(context.Background(), "acme", repoRoot, storeRoot, true)
	if err != nil {
		t.Fatalf("Add() error = %v", err)
	}

	if org.Name != "acme" {
		t.Fatalf("org.Name = %q, want acme", org.Name)
	}
	if org.AuthPolicy.SessionTTLMinutes != 15 {
		t.Fatalf("session ttl = %d, want 15", org.AuthPolicy.SessionTTLMinutes)
	}

	cfg, err := manager.Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.ActiveOrganization != "acme" {
		t.Fatalf("active org = %q, want acme", cfg.ActiveOrganization)
	}
	if _, ok := cfg.Organizations["acme"]; !ok {
		t.Fatalf("organization was not persisted")
	}
}

func TestAddOrganizationExpandsHomeRelativePaths(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	homeDir, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("UserHomeDir() error = %v", err)
	}
	repoRoot := filepath.Join(homeDir, "repos", "acme")
	if err := os.MkdirAll(repoRoot, 0o755); err != nil {
		t.Fatalf("mkdir repo root: %v", err)
	}

	manager := config.NewManagerWithPath(filepath.Join(t.TempDir(), "config.json"))
	org, err := NewService(manager).Add(context.Background(), "acme", "~/repos/acme", "~/secrets/acme", true)
	if err != nil {
		t.Fatalf("Add() error = %v", err)
	}

	if org.RepoRoot != repoRoot {
		t.Fatalf("RepoRoot = %q, want %q", org.RepoRoot, repoRoot)
	}
	wantStoreRoot := filepath.Join(homeDir, "secrets", "acme")
	if org.StoreRoot != wantStoreRoot {
		t.Fatalf("StoreRoot = %q, want %q", org.StoreRoot, wantStoreRoot)
	}
}

func TestSetActiveOrganization(t *testing.T) {
	t.Parallel()

	tempDir := t.TempDir()
	repoRoot := filepath.Join(tempDir, "repos")
	if err := os.MkdirAll(repoRoot, 0o755); err != nil {
		t.Fatalf("mkdir repo root: %v", err)
	}

	manager := config.NewManagerWithPath(filepath.Join(tempDir, "config.json"))
	if err := manager.Save(&config.Config{
		Version:            1,
		ActiveOrganization: "acme",
		Organizations: map[string]config.Organization{
			"acme":  {Name: "acme", RepoRoot: repoRoot, StoreRoot: filepath.Join(tempDir, "store-acme")},
			"other": {Name: "other", RepoRoot: repoRoot, StoreRoot: filepath.Join(tempDir, "store-other")},
		},
	}); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	org, err := NewService(manager).SetActive(context.Background(), "other")
	if err != nil {
		t.Fatalf("SetActive() error = %v", err)
	}
	if org.Name != "other" {
		t.Fatalf("org.Name = %q, want other", org.Name)
	}

	cfg, err := manager.Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.ActiveOrganization != "other" {
		t.Fatalf("active org = %q, want other", cfg.ActiveOrganization)
	}
}
