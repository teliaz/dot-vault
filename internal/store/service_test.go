package store

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/teliaz/dot-vault/internal/config"
)

type fakeKeyProvider struct {
	key []byte
}

type failingKeyProvider struct{}

func (f failingKeyProvider) GetOrCreateMasterKey(_ context.Context, _ string) ([]byte, error) {
	return nil, fmt.Errorf("key provider should not be called")
}

func TestBackupCreatesSnapshotOnlyWhenContentChanged(t *testing.T) {
	t.Parallel()

	tempDir := t.TempDir()
	manager := newTestConfigManager(t, tempDir)
	service := NewService(manager, fakeKeyProvider{
		key: []byte("0123456789abcdef0123456789abcdef"),
	})

	if _, err := service.Put(context.Background(), PutInput{
		Repository: "app-one",
		EnvFile:    ".env",
		SourcePath: "/repos/app-one/.env",
		Plaintext:  []byte("API_KEY=one\n"),
	}); err != nil {
		t.Fatalf("Put() error = %v", err)
	}

	firstBackup, err := service.Backup(context.Background(), BackupInput{
		Repository: "app-one",
		EnvFile:    ".env",
	})
	if err != nil {
		t.Fatalf("Backup() error = %v", err)
	}
	if !firstBackup.Created {
		t.Fatalf("first backup Created = false, want true")
	}
	if _, err := os.Stat(firstBackup.BackupPath); err != nil {
		t.Fatalf("backup file was not written: %v", err)
	}

	secondBackup, err := service.Backup(context.Background(), BackupInput{
		Repository: "app-one",
		EnvFile:    ".env",
	})
	if err != nil {
		t.Fatalf("Backup() second error = %v", err)
	}
	if secondBackup.Created {
		t.Fatalf("second backup Created = true, want false")
	}

	if _, err := service.Put(context.Background(), PutInput{
		Repository: "app-one",
		EnvFile:    ".env",
		SourcePath: "/repos/app-one/.env",
		Plaintext:  []byte("API_KEY=two\n"),
	}); err != nil {
		t.Fatalf("Put() changed error = %v", err)
	}

	thirdBackup, err := service.Backup(context.Background(), BackupInput{
		Repository: "app-one",
		EnvFile:    ".env",
	})
	if err != nil {
		t.Fatalf("Backup() third error = %v", err)
	}
	if !thirdBackup.Created {
		t.Fatalf("third backup Created = false, want true")
	}

	backups, err := service.ListBackups("", "app-one", ".env")
	if err != nil {
		t.Fatalf("ListBackups() error = %v", err)
	}
	if len(backups) != 2 {
		t.Fatalf("len(backups) = %d, want 2", len(backups))
	}
}

func TestBackupRecreatesMissingBackupDirectory(t *testing.T) {
	t.Parallel()

	tempDir := t.TempDir()
	manager := newTestConfigManager(t, tempDir)
	service := NewService(manager, fakeKeyProvider{
		key: []byte("0123456789abcdef0123456789abcdef"),
	})

	if _, err := service.Put(context.Background(), PutInput{
		Repository: "app-one",
		EnvFile:    ".env",
		SourcePath: "/repos/app-one/.env",
		Plaintext:  []byte("API_KEY=one\n"),
	}); err != nil {
		t.Fatalf("Put() error = %v", err)
	}

	if err := os.RemoveAll(filepath.Join(tempDir, "store", "backups")); err != nil {
		t.Fatalf("RemoveAll(backups) error = %v", err)
	}

	backup, err := service.Backup(context.Background(), BackupInput{
		Repository: "app-one",
		EnvFile:    ".env",
	})
	if err != nil {
		t.Fatalf("Backup() error = %v", err)
	}
	if !backup.Created {
		t.Fatalf("backup Created = false, want true")
	}
	if _, err := os.Stat(backup.BackupPath); err != nil {
		t.Fatalf("backup file was not written after deleting backup directory: %v", err)
	}
}

func newTestConfigManager(t *testing.T, tempDir string) *config.Manager {
	t.Helper()

	manager := config.NewManagerWithPath(filepath.Join(tempDir, "config.json"))
	cfg := &config.Config{
		Version:            1,
		ActiveOrganization: "acme",
		Organizations: map[string]config.Organization{
			"acme": {
				Name:      "acme",
				RepoRoot:  filepath.Join(tempDir, "repos"),
				StoreRoot: filepath.Join(tempDir, "store"),
			},
		},
	}
	if err := manager.Save(cfg); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	return manager
}

func (f fakeKeyProvider) GetOrCreateMasterKey(_ context.Context, _ string) ([]byte, error) {
	return f.key, nil
}

func TestMetadataDoesNotRequireMasterKey(t *testing.T) {
	t.Parallel()

	tempDir := t.TempDir()
	manager := newTestConfigManager(t, tempDir)
	service := NewService(manager, fakeKeyProvider{
		key: []byte("0123456789abcdef0123456789abcdef"),
	})

	metadata, err := service.Put(context.Background(), PutInput{
		Repository: "app-one",
		EnvFile:    ".env",
		SourcePath: "/repos/app-one/.env",
		Plaintext:  []byte("API_KEY=one\n"),
	})
	if err != nil {
		t.Fatalf("Put() error = %v", err)
	}

	metadataOnlyService := NewService(manager, failingKeyProvider{})
	loadedMetadata, err := metadataOnlyService.Metadata(GetInput{
		Repository: "app-one",
		EnvFile:    ".env",
	})
	if err != nil {
		t.Fatalf("Metadata() error = %v", err)
	}
	if loadedMetadata.ContentFingerprint != metadata.ContentFingerprint {
		t.Fatalf("fingerprint = %q, want %q", loadedMetadata.ContentFingerprint, metadata.ContentFingerprint)
	}
}

func TestPutGetRoundTrip(t *testing.T) {
	t.Parallel()

	tempDir := t.TempDir()
	manager := config.NewManagerWithPath(filepath.Join(tempDir, "config.json"))
	cfg := &config.Config{
		Version:            1,
		ActiveOrganization: "acme",
		Organizations: map[string]config.Organization{
			"acme": {
				Name:      "acme",
				RepoRoot:  filepath.Join(tempDir, "repos"),
				StoreRoot: filepath.Join(tempDir, "store"),
			},
		},
	}
	if err := manager.Save(cfg); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	service := NewService(manager, fakeKeyProvider{
		key: []byte("0123456789abcdef0123456789abcdef"),
	})

	input := PutInput{
		Repository: "app-one",
		EnvFile:    ".env.production",
		SourcePath: "/repos/app-one/.env.production",
		Plaintext:  []byte("API_KEY=super-secret\n"),
	}
	metadata, err := service.Put(context.Background(), input)
	if err != nil {
		t.Fatalf("Put() error = %v", err)
	}

	plaintext, loadedMetadata, err := service.Get(context.Background(), GetInput{
		Repository: "app-one",
		EnvFile:    ".env.production",
	})
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}

	if string(plaintext) != string(input.Plaintext) {
		t.Fatalf("plaintext = %q, want %q", plaintext, input.Plaintext)
	}
	if loadedMetadata.ContentFingerprint != metadata.ContentFingerprint {
		t.Fatalf("fingerprint = %q, want %q", loadedMetadata.ContentFingerprint, metadata.ContentFingerprint)
	}
}
