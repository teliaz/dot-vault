package cmd

import (
	"path/filepath"
	"testing"
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
