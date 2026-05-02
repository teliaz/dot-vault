package cmd

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/teliaz/dot-vault/internal/config"
	"github.com/teliaz/dot-vault/internal/crypto"
	"github.com/teliaz/dot-vault/internal/orgs"
	"github.com/teliaz/dot-vault/internal/store"
)

func TestSafeRepoEnvPath(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	targetPath, err := safeRepoEnvPath(repoRoot, "app", ".env")
	if err != nil {
		t.Fatalf("safeRepoEnvPath() error = %v", err)
	}

	want := filepath.Join(repoRoot, "app", ".env")
	if targetPath != want {
		t.Fatalf("targetPath = %q, want %q", targetPath, want)
	}
}

func TestSafeRepoEnvPathRejectsEscapes(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	if _, err := safeRepoEnvPath(repoRoot, "../outside", ".env"); err == nil {
		t.Fatalf("safeRepoEnvPath() error = nil, want error")
	}
	if _, err := safeRepoEnvPath(repoRoot, "app", "../.env"); err == nil {
		t.Fatalf("safeRepoEnvPath() env error = nil, want error")
	}
}

func TestCollectRepoStatusRowsIncludesStoredRecordsWithoutCheckout(t *testing.T) {
	t.Parallel()

	tempDir := t.TempDir()
	repoRoot := filepath.Join(tempDir, "repos")
	storeRoot := filepath.Join(tempDir, "store")
	configPath := filepath.Join(tempDir, "config.json")
	if err := os.MkdirAll(filepath.Join(repoRoot, "present", ".git"), 0o755); err != nil {
		t.Fatalf("mkdir present repo: %v", err)
	}
	if err := os.WriteFile(filepath.Join(repoRoot, "present", ".env"), []byte("API_KEY=current\n"), 0o600); err != nil {
		t.Fatalf("write present env: %v", err)
	}

	manager := config.NewManagerWithPath(configPath)
	if err := manager.Save(&config.Config{
		Version:            1,
		ActiveOrganization: "acme",
		Organizations: map[string]config.Organization{
			"acme": {Name: "acme", RepoRoot: repoRoot, StoreRoot: storeRoot},
		},
	}); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	keyProvider := crypto.NewKeyProvider("dot-vault-test")
	if err := keyProvider.SetPassphrase("acme", "correct horse battery staple"); err != nil {
		t.Fatalf("SetPassphrase() error = %v", err)
	}
	storeService := store.NewService(manager, keyProvider)
	if _, err := storeService.Put(context.Background(), store.PutInput{
		Organization: "acme",
		Repository:   "missing",
		EnvFile:      ".env",
		SourcePath:   filepath.Join(repoRoot, "missing", ".env"),
		Plaintext:    []byte("API_KEY=stored\n"),
	}); err != nil {
		t.Fatalf("Put() missing repo error = %v", err)
	}

	app := &appContext{
		configManager: manager,
		orgService:    orgs.NewService(manager),
		storeService:  storeService,
		keyProvider:   keyProvider,
	}
	rows, err := collectRepoStatusRows(context.Background(), app, "acme", "")
	if err != nil {
		t.Fatalf("collectRepoStatusRows() error = %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("len(rows) = %d, want 2: %#v", len(rows), rows)
	}

	var foundMissing bool
	for _, row := range rows {
		if row.Repo != "missing" {
			continue
		}
		foundMissing = true
		if row.DriftStatus != "repo_missing" {
			t.Fatalf("missing repo DriftStatus = %q, want repo_missing", row.DriftStatus)
		}
		if row.GitPresent {
			t.Fatalf("missing repo GitPresent = true, want false")
		}
		if row.StoreMissing {
			t.Fatalf("missing repo StoreMissing = true, want false")
		}
	}
	if !foundMissing {
		t.Fatalf("stored missing repository row was not included: %#v", rows)
	}
}

func TestCollectRepoStatusRowsIncludesRepositoriesWithoutEnvFiles(t *testing.T) {
	t.Parallel()

	tempDir := t.TempDir()
	repoRoot := filepath.Join(tempDir, "repos")
	storeRoot := filepath.Join(tempDir, "store")
	configPath := filepath.Join(tempDir, "config.json")
	if err := os.MkdirAll(filepath.Join(repoRoot, "empty-repo", ".git"), 0o755); err != nil {
		t.Fatalf("mkdir empty repo: %v", err)
	}

	manager := config.NewManagerWithPath(configPath)
	if err := manager.Save(&config.Config{
		Version:            1,
		ActiveOrganization: "acme",
		Organizations: map[string]config.Organization{
			"acme": {Name: "acme", RepoRoot: repoRoot, StoreRoot: storeRoot},
		},
	}); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	app := &appContext{
		configManager: manager,
		orgService:    orgs.NewService(manager),
		storeService:  store.NewService(manager, crypto.NewKeyProvider("dot-vault-test")),
	}
	rows, err := collectRepoStatusRows(context.Background(), app, "acme", "")
	if err != nil {
		t.Fatalf("collectRepoStatusRows() error = %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("len(rows) = %d, want 1: %#v", len(rows), rows)
	}
	row := rows[0]
	if !row.RepositoryOnly {
		t.Fatalf("RepositoryOnly = false, want true")
	}
	if row.Repo != "empty-repo" || row.EnvFile != "" {
		t.Fatalf("row = %#v, want empty-repo with blank env", row)
	}
	if row.DriftStatus != "no_env" {
		t.Fatalf("DriftStatus = %q, want no_env", row.DriftStatus)
	}
}

func TestCollectRepoStatusRowsSuggestsEnvFromSample(t *testing.T) {
	t.Parallel()

	tempDir := t.TempDir()
	repoRoot := filepath.Join(tempDir, "repos")
	storeRoot := filepath.Join(tempDir, "store")
	configPath := filepath.Join(tempDir, "config.json")
	if err := os.MkdirAll(filepath.Join(repoRoot, "sampled-repo", ".git"), 0o755); err != nil {
		t.Fatalf("mkdir sampled repo: %v", err)
	}
	if err := os.WriteFile(filepath.Join(repoRoot, "sampled-repo", ".env_sample"), []byte("API_KEY=\n"), 0o600); err != nil {
		t.Fatalf("write sample env: %v", err)
	}

	manager := config.NewManagerWithPath(configPath)
	if err := manager.Save(&config.Config{
		Version:            1,
		ActiveOrganization: "acme",
		Organizations: map[string]config.Organization{
			"acme": {Name: "acme", RepoRoot: repoRoot, StoreRoot: storeRoot},
		},
	}); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	app := &appContext{
		configManager: manager,
		orgService:    orgs.NewService(manager),
		storeService:  store.NewService(manager, crypto.NewKeyProvider("dot-vault-test")),
	}
	rows, err := collectRepoStatusRows(context.Background(), app, "acme", "")
	if err != nil {
		t.Fatalf("collectRepoStatusRows() error = %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("len(rows) = %d, want 1: %#v", len(rows), rows)
	}
	row := rows[0]
	if !row.RepositoryOnly {
		t.Fatalf("RepositoryOnly = false, want true")
	}
	if row.Repo != "sampled-repo" || row.EnvFile != ".env" {
		t.Fatalf("row = %#v, want sampled-repo/.env suggestion", row)
	}
	if row.DriftStatus != "env_suggested" {
		t.Fatalf("DriftStatus = %q, want env_suggested", row.DriftStatus)
	}
	if row.EnvSuggestedFrom != ".env_sample" {
		t.Fatalf("EnvSuggestedFrom = %q, want .env_sample", row.EnvSuggestedFrom)
	}
	if row.CurrentAt != "create from .env_sample" {
		t.Fatalf("CurrentAt = %q, want create from .env_sample", row.CurrentAt)
	}
}
