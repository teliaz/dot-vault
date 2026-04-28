package orgs

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/teliaz/dot-vault/internal/config"
)

func TestScanDiscoversRepositoriesAndEnvFiles(t *testing.T) {
	t.Parallel()

	tempDir := t.TempDir()
	repoRoot := filepath.Join(tempDir, "repos")
	storeRoot := filepath.Join(tempDir, "store")
	appRepo := filepath.Join(repoRoot, "app")
	nestedRepo := filepath.Join(repoRoot, "team", "api")

	mkdirAll(t, filepath.Join(appRepo, ".git"))
	mkdirAll(t, filepath.Join(nestedRepo, ".git"))
	writeFile(t, filepath.Join(appRepo, ".env"), "APP_SECRET=one\n")
	writeFile(t, filepath.Join(appRepo, ".env.local"), "LOCAL_SECRET=two\n")
	writeFile(t, filepath.Join(appRepo, ".env.example"), "SAMPLE=no\n")
	writeFile(t, filepath.Join(appRepo, ".gitignore"), ".env.example\n")
	writeFile(t, filepath.Join(nestedRepo, ".env.production"), "API_SECRET=three\n")

	manager := config.NewManagerWithPath(filepath.Join(tempDir, "config.json"))
	if err := manager.Save(&config.Config{
		Version:            1,
		ActiveOrganization: "acme",
		Organizations: map[string]config.Organization{
			"acme": {
				Name:      "acme",
				RepoRoot:  repoRoot,
				StoreRoot: storeRoot,
			},
		},
	}); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	repos, err := NewService(manager).Scan(context.Background(), "")
	if err != nil {
		t.Fatalf("Scan() error = %v", err)
	}

	if len(repos) != 2 {
		t.Fatalf("len(repos) = %d, want 2", len(repos))
	}
	if repos[0].RelPath != "app" {
		t.Fatalf("first repo = %q, want app", repos[0].RelPath)
	}
	envNames := []string{}
	for _, envFile := range repos[0].EnvFiles {
		envNames = append(envNames, envFile.Name)
	}
	want := []string{".env", ".env.example", ".env.local"}
	for i := range want {
		if envNames[i] != want[i] {
			t.Fatalf("envNames = %#v, want %#v", envNames, want)
		}
	}
	if repos[1].RelPath != "team/api" {
		t.Fatalf("second repo = %q, want team/api", repos[1].RelPath)
	}
}

func TestDiscoverEnvFilesExcludesSamplesUnlessIgnored(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	writeFile(t, filepath.Join(repoRoot, ".env"), "SECRET=one\n")
	writeFile(t, filepath.Join(repoRoot, ".env.example"), "SECRET=sample\n")

	envFiles, err := discoverEnvFiles(repoRoot)
	if err != nil {
		t.Fatalf("discoverEnvFiles() error = %v", err)
	}
	if len(envFiles) != 1 || envFiles[0].Name != ".env" {
		t.Fatalf("envFiles = %#v, want only .env", envFiles)
	}
}

func mkdirAll(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatalf("MkdirAll(%q) error = %v", path, err)
	}
}

func writeFile(t *testing.T, path string, contents string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll(%q) error = %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatalf("WriteFile(%q) error = %v", path, err)
	}
}
