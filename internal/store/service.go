package store

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/teliaz/dot-vault/internal/config"
	"github.com/teliaz/dot-vault/internal/crypto"
)

const keyVersion = 1

type masterKeyProvider interface {
	GetOrCreateMasterKey(ctx context.Context, organization string) ([]byte, error)
}

type Service struct {
	configManager *config.Manager
	keyProvider   masterKeyProvider
}

type PutInput struct {
	Organization string
	Repository   string
	EnvFile      string
	SourcePath   string
	RemoteURL    string
	Plaintext    []byte
}

type GetInput struct {
	Organization string
	Repository   string
	EnvFile      string
}

type BackupInput struct {
	Organization string
	Repository   string
	EnvFile      string
	RemoteURL    string
}

type BackupResult struct {
	Metadata   Metadata
	Created    bool
	BackupPath string
}

type Metadata struct {
	Repository            string     `json:"repository"`
	EnvFile               string     `json:"env_file"`
	SourcePath            string     `json:"source_path"`
	RemoteURL             string     `json:"remote_url,omitempty"`
	LastImportedAt        time.Time  `json:"last_imported_at"`
	LastBackupAt          *time.Time `json:"last_backup_at,omitempty"`
	LastBackupFingerprint string     `json:"last_backup_fingerprint,omitempty"`
	ContentFingerprint    string     `json:"content_fingerprint"`
	KeyVersion            int        `json:"key_version"`
}

type envelope struct {
	Version         int      `json:"version"`
	Cipher          string   `json:"cipher"`
	WrappedKey      string   `json:"wrapped_key"`
	WrappedKeyNonce string   `json:"wrapped_key_nonce"`
	DataNonce       string   `json:"data_nonce"`
	Ciphertext      string   `json:"ciphertext"`
	Metadata        Metadata `json:"metadata"`
}

func NewService(configManager *config.Manager, keyProvider masterKeyProvider) *Service {
	return &Service{
		configManager: configManager,
		keyProvider:   keyProvider,
	}
}

func (s *Service) Put(ctx context.Context, input PutInput) (Metadata, error) {
	org, err := s.resolveOrganization(input.Organization)
	if err != nil {
		return Metadata{}, err
	}

	repository, envFile, err := normalizeIdentifiers(input.Repository, input.EnvFile)
	if err != nil {
		return Metadata{}, err
	}
	if strings.TrimSpace(input.SourcePath) == "" {
		return Metadata{}, fmt.Errorf("source path is required")
	}

	masterKey, err := s.keyProvider.GetOrCreateMasterKey(ctx, org.Name)
	if err != nil {
		return Metadata{}, err
	}

	fileKey := make([]byte, 32)
	if _, err := rand.Read(fileKey); err != nil {
		return Metadata{}, fmt.Errorf("generate file key: %w", err)
	}

	ciphertext, dataNonce, err := encrypt(fileKey, input.Plaintext)
	if err != nil {
		return Metadata{}, err
	}
	wrappedKey, wrappedNonce, err := encrypt(masterKey, fileKey)
	if err != nil {
		return Metadata{}, err
	}

	metadata := Metadata{
		Repository:         repository,
		EnvFile:            envFile,
		SourcePath:         input.SourcePath,
		RemoteURL:          strings.TrimSpace(input.RemoteURL),
		LastImportedAt:     time.Now().UTC(),
		ContentFingerprint: Fingerprint(input.Plaintext),
		KeyVersion:         keyVersion,
	}
	existingRecord, err := s.readEnvelope(org.StoreRoot, repository, envFile)
	if err == nil {
		metadata.LastBackupAt = existingRecord.Metadata.LastBackupAt
		metadata.LastBackupFingerprint = existingRecord.Metadata.LastBackupFingerprint
		if metadata.RemoteURL == "" {
			metadata.RemoteURL = existingRecord.Metadata.RemoteURL
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return Metadata{}, err
	}

	record := envelope{
		Version:         1,
		Cipher:          "aes-256-gcm",
		WrappedKey:      base64.StdEncoding.EncodeToString(wrappedKey),
		WrappedKeyNonce: base64.StdEncoding.EncodeToString(wrappedNonce),
		DataNonce:       base64.StdEncoding.EncodeToString(dataNonce),
		Ciphertext:      base64.StdEncoding.EncodeToString(ciphertext),
		Metadata:        metadata,
	}

	recordPath := s.recordPath(org.StoreRoot, repository, envFile)
	if err := os.MkdirAll(filepath.Dir(recordPath), 0o700); err != nil {
		return Metadata{}, fmt.Errorf("create record directory: %w", err)
	}

	payload, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		return Metadata{}, fmt.Errorf("encode encrypted record: %w", err)
	}
	payload = append(payload, '\n')

	if err := os.WriteFile(recordPath, payload, 0o600); err != nil {
		return Metadata{}, fmt.Errorf("write encrypted record: %w", err)
	}

	return metadata, nil
}

func (s *Service) Get(ctx context.Context, input GetInput) ([]byte, Metadata, error) {
	org, err := s.resolveOrganization(input.Organization)
	if err != nil {
		return nil, Metadata{}, err
	}

	repository, envFile, err := normalizeIdentifiers(input.Repository, input.EnvFile)
	if err != nil {
		return nil, Metadata{}, err
	}

	record, err := s.readEnvelope(org.StoreRoot, repository, envFile)
	if err != nil {
		return nil, Metadata{}, err
	}

	masterKey, err := s.keyProvider.GetOrCreateMasterKey(ctx, org.Name)
	if err != nil {
		return nil, Metadata{}, err
	}

	wrappedKey, err := base64.StdEncoding.DecodeString(record.WrappedKey)
	if err != nil {
		return nil, Metadata{}, fmt.Errorf("decode wrapped key: %w", err)
	}
	wrappedNonce, err := base64.StdEncoding.DecodeString(record.WrappedKeyNonce)
	if err != nil {
		return nil, Metadata{}, fmt.Errorf("decode wrapped key nonce: %w", err)
	}

	fileKey, err := decrypt(masterKey, wrappedKey, wrappedNonce)
	if err != nil {
		return nil, Metadata{}, err
	}

	ciphertext, err := base64.StdEncoding.DecodeString(record.Ciphertext)
	if err != nil {
		return nil, Metadata{}, fmt.Errorf("decode ciphertext: %w", err)
	}
	dataNonce, err := base64.StdEncoding.DecodeString(record.DataNonce)
	if err != nil {
		return nil, Metadata{}, fmt.Errorf("decode data nonce: %w", err)
	}

	plaintext, err := decrypt(fileKey, ciphertext, dataNonce)
	if err != nil {
		return nil, Metadata{}, err
	}

	return plaintext, record.Metadata, nil
}

func (s *Service) Metadata(input GetInput) (Metadata, error) {
	org, err := s.resolveOrganization(input.Organization)
	if err != nil {
		return Metadata{}, err
	}

	repository, envFile, err := normalizeIdentifiers(input.Repository, input.EnvFile)
	if err != nil {
		return Metadata{}, err
	}

	record, err := s.readEnvelope(org.StoreRoot, repository, envFile)
	if err != nil {
		return Metadata{}, err
	}
	return record.Metadata, nil
}

func (s *Service) ListMetadata(organization string) ([]Metadata, error) {
	org, err := s.resolveOrganization(organization)
	if err != nil {
		return nil, err
	}

	reposDir := filepath.Join(org.StoreRoot, "repos")
	var records []Metadata
	err = filepath.WalkDir(reposDir, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".enc.json") {
			return nil
		}

		payload, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("read encrypted record %s: %w", path, err)
		}
		var record envelope
		if err := json.Unmarshal(payload, &record); err != nil {
			return fmt.Errorf("decode encrypted record %s: %w", path, err)
		}
		records = append(records, record.Metadata)
		return nil
	})
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("list encrypted records: %w", err)
	}

	sort.Slice(records, func(i, j int) bool {
		if records[i].Repository == records[j].Repository {
			return records[i].EnvFile < records[j].EnvFile
		}
		return records[i].Repository < records[j].Repository
	})
	return records, nil
}

func (s *Service) Backup(_ context.Context, input BackupInput) (BackupResult, error) {
	org, err := s.resolveOrganization(input.Organization)
	if err != nil {
		return BackupResult{}, err
	}

	repository, envFile, err := normalizeIdentifiers(input.Repository, input.EnvFile)
	if err != nil {
		return BackupResult{}, err
	}

	record, err := s.readEnvelope(org.StoreRoot, repository, envFile)
	if err != nil {
		return BackupResult{}, err
	}

	if remoteURL := strings.TrimSpace(input.RemoteURL); remoteURL != "" {
		record.Metadata.RemoteURL = remoteURL
	}

	if record.Metadata.LastBackupFingerprint == record.Metadata.ContentFingerprint {
		if strings.TrimSpace(input.RemoteURL) != "" {
			if err := s.writeEnvelope(s.recordPath(org.StoreRoot, repository, envFile), record); err != nil {
				return BackupResult{}, err
			}
		}
		return BackupResult{
			Metadata: record.Metadata,
			Created:  false,
		}, nil
	}

	now := time.Now().UTC()
	record.Metadata.LastBackupAt = &now
	record.Metadata.LastBackupFingerprint = record.Metadata.ContentFingerprint

	backupPath := s.backupPath(org.StoreRoot, repository, envFile, now)
	if err := os.MkdirAll(filepath.Dir(backupPath), 0o700); err != nil {
		return BackupResult{}, fmt.Errorf("create backup directory: %w", err)
	}
	if err := s.writeEnvelope(backupPath, record); err != nil {
		return BackupResult{}, fmt.Errorf("write backup record: %w", err)
	}
	if err := s.writeEnvelope(s.recordPath(org.StoreRoot, repository, envFile), record); err != nil {
		return BackupResult{}, err
	}

	return BackupResult{
		Metadata:   record.Metadata,
		Created:    true,
		BackupPath: backupPath,
	}, nil
}

func (s *Service) ListBackups(organization string, repository string, envFile string) ([]string, error) {
	org, err := s.resolveOrganization(organization)
	if err != nil {
		return nil, err
	}

	repository, envFile, err = normalizeIdentifiers(repository, envFile)
	if err != nil {
		return nil, err
	}

	dir := s.backupDir(org.StoreRoot, repository, envFile)
	entries, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read backup directory: %w", err)
	}

	backups := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".enc.json") {
			continue
		}
		backups = append(backups, filepath.Join(dir, entry.Name()))
	}
	sort.Strings(backups)
	return backups, nil
}

func (s *Service) ResetBackups(organization string) (int, error) {
	org, err := s.resolveOrganization(organization)
	if err != nil {
		return 0, err
	}

	records, err := s.ListMetadata(org.Name)
	if err != nil {
		return 0, err
	}

	reset := 0
	for _, metadata := range records {
		record, err := s.readEnvelope(org.StoreRoot, metadata.Repository, metadata.EnvFile)
		if err != nil {
			return reset, err
		}
		if record.Metadata.LastBackupAt == nil && record.Metadata.LastBackupFingerprint == "" {
			continue
		}
		record.Metadata.LastBackupAt = nil
		record.Metadata.LastBackupFingerprint = ""
		if err := s.writeEnvelope(s.recordPath(org.StoreRoot, metadata.Repository, metadata.EnvFile), record); err != nil {
			return reset, err
		}
		reset++
	}

	if err := os.RemoveAll(filepath.Join(org.StoreRoot, "backups")); err != nil {
		return reset, fmt.Errorf("remove backups directory: %w", err)
	}
	return reset, nil
}

func (s *Service) resolveOrganization(name string) (config.Organization, error) {
	cfg, err := s.configManager.Load()
	if err != nil {
		return config.Organization{}, err
	}

	targetName := strings.TrimSpace(name)
	if targetName == "" {
		targetName = cfg.ActiveOrganization
	}
	if targetName == "" {
		return config.Organization{}, fmt.Errorf("organization is required; no active organization is configured")
	}

	org, ok := cfg.Organizations[targetName]
	if !ok {
		return config.Organization{}, fmt.Errorf("organization %q not found", targetName)
	}
	return org, nil
}

func normalizeIdentifiers(repository string, envFile string) (string, string, error) {
	repo := filepath.Clean(strings.TrimSpace(repository))
	env := strings.TrimSpace(envFile)
	if repo == "" || repo == "." {
		return "", "", fmt.Errorf("repository is required")
	}
	if env == "" {
		return "", "", fmt.Errorf("env file is required")
	}
	if strings.Contains(env, "/") {
		return "", "", fmt.Errorf("env file must be a file name, not a path")
	}
	return repo, env, nil
}

func (s *Service) recordPath(storeRoot string, repository string, envFile string) string {
	repoHash := sha256.Sum256([]byte(repository))
	repoDir := filepath.Join(storeRoot, "repos", sanitizePathSegment(filepath.Base(repository))+"-"+hex.EncodeToString(repoHash[:8]))
	fileName := sanitizePathSegment(envFile) + ".enc.json"
	return filepath.Join(repoDir, fileName)
}

func (s *Service) backupPath(storeRoot string, repository string, envFile string, timestamp time.Time) string {
	return filepath.Join(s.backupDir(storeRoot, repository, envFile), timestamp.Format("20060102T150405.000000000Z")+".enc.json")
}

func (s *Service) backupDir(storeRoot string, repository string, envFile string) string {
	repoHash := sha256.Sum256([]byte(repository))
	return filepath.Join(
		storeRoot,
		"backups",
		sanitizePathSegment(filepath.Base(repository))+"-"+hex.EncodeToString(repoHash[:8]),
		sanitizePathSegment(envFile),
	)
}

func (s *Service) readEnvelope(storeRoot string, repository string, envFile string) (envelope, error) {
	recordPath := s.recordPath(storeRoot, repository, envFile)
	payload, err := os.ReadFile(recordPath)
	if err != nil {
		return envelope{}, fmt.Errorf("read encrypted record: %w", err)
	}

	var record envelope
	if err := json.Unmarshal(payload, &record); err != nil {
		return envelope{}, fmt.Errorf("decode encrypted record: %w", err)
	}
	return record, nil
}

func (s *Service) writeEnvelope(path string, record envelope) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("create encrypted record directory: %w", err)
	}

	payload, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		return fmt.Errorf("encode encrypted record: %w", err)
	}
	payload = append(payload, '\n')

	if err := os.WriteFile(path, payload, 0o600); err != nil {
		return fmt.Errorf("write encrypted record: %w", err)
	}
	return nil
}

func sanitizePathSegment(value string) string {
	replacer := strings.NewReplacer("/", "-", "\\", "-", " ", "-", "..", "-")
	sanitized := replacer.Replace(strings.TrimSpace(value))
	if sanitized == "" {
		return "item"
	}
	return sanitized
}

func Fingerprint(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func encrypt(key []byte, plaintext []byte) ([]byte, []byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, nil, fmt.Errorf("create cipher: %w", err)
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, nil, fmt.Errorf("create AEAD: %w", err)
	}
	nonce := make([]byte, aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, nil, fmt.Errorf("generate nonce: %w", err)
	}
	return aead.Seal(nil, nonce, plaintext, nil), nonce, nil
}

func decrypt(key []byte, ciphertext []byte, nonce []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("create cipher: %w", err)
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("create AEAD: %w", err)
	}
	plaintext, err := aead.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, fmt.Errorf("decrypt payload: %w", err)
	}
	return plaintext, nil
}

var _ masterKeyProvider = (*crypto.KeyProvider)(nil)
