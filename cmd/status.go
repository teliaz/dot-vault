package cmd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/teliaz/dot-vault/internal/config"
	"github.com/teliaz/dot-vault/internal/store"
)

type repoStatusRow struct {
	Organization   string
	Repo           string
	EnvFile        string
	DriftStatus    string
	BackupStatus   string
	ImportedAt     string
	BackupAt       string
	CurrentAt      string
	RemoteURL      string
	GitPresent     bool
	EnvPresent     bool
	StoreMissing   bool
	RepositoryOnly bool
	DiffSummary    string
}

func collectRepoStatusRows(ctx context.Context, app *appContext, orgName string, repoFilter string) ([]repoStatusRow, error) {
	org, err := app.orgService.ResolveOrganization(orgName)
	if err != nil {
		return nil, err
	}

	repos, err := app.orgService.Scan(ctx, orgName)
	if err != nil {
		return nil, err
	}
	repos = filterRepositories(repos, repoFilter)

	metadataRecords, err := app.storeService.ListMetadata(org.Name)
	if err != nil {
		return nil, err
	}

	rowsByKey := map[string]repoStatusRow{}
	for _, repo := range repos {
		if len(repo.EnvFiles) == 0 {
			rowsByKey[statusRowKey(repo.RelPath, "")] = repoStatusRow{
				Organization:   org.Name,
				Repo:           repo.RelPath,
				EnvFile:        "",
				DriftStatus:    "no_env",
				BackupStatus:   "none",
				CurrentAt:      "no env",
				GitPresent:     true,
				EnvPresent:     false,
				StoreMissing:   true,
				RepositoryOnly: true,
			}
			continue
		}

		for _, envFile := range repo.EnvFiles {
			payload, err := os.ReadFile(envFile.AbsPath)
			if err != nil {
				return nil, fmt.Errorf("read %s: %w", envFile.AbsPath, err)
			}

			row := repoStatusRow{
				Organization: org.Name,
				Repo:         repo.RelPath,
				EnvFile:      envFile.Name,
				CurrentAt:    formatTimestamp(envFile.UpdatedAt),
				GitPresent:   true,
				EnvPresent:   true,
			}

			metadata, err := app.storeService.Metadata(store.GetInput{
				Organization: org.Name,
				Repository:   repo.RelPath,
				EnvFile:      envFile.Name,
			})
			if errors.Is(err, os.ErrNotExist) {
				row.StoreMissing = true
				row.DriftStatus = "missing"
				row.BackupStatus = "none"
				rowsByKey[statusRowKey(row.Repo, row.EnvFile)] = row
				continue
			}
			if err != nil {
				return nil, err
			}

			applyStoredMetadata(&row, metadata)
			if store.Fingerprint(payload) != metadata.ContentFingerprint {
				row.DriftStatus = "drift"
			}
			rowsByKey[statusRowKey(row.Repo, row.EnvFile)] = row
		}
	}

	for _, metadata := range metadataRecords {
		if repoFilter != "" && metadata.Repository != repoFilter && filepath.Base(metadata.Repository) != repoFilter {
			continue
		}
		key := statusRowKey(metadata.Repository, metadata.EnvFile)
		if _, exists := rowsByKey[key]; exists {
			continue
		}
		delete(rowsByKey, statusRowKey(metadata.Repository, ""))

		row := repoStatusRow{
			Organization: org.Name,
			Repo:         metadata.Repository,
			EnvFile:      metadata.EnvFile,
			GitPresent:   gitPresent(org, metadata.Repository),
			EnvPresent:   false,
			CurrentAt:    "missing",
		}
		applyStoredMetadata(&row, metadata)
		if row.GitPresent {
			row.DriftStatus = "env_missing"
		} else {
			row.DriftStatus = "repo_missing"
		}
		rowsByKey[key] = row
	}

	if len(rowsByKey) == 0 {
		if repoFilter != "" {
			return nil, fmt.Errorf("no repositories matched")
		}
		return nil, nil
	}

	rows := make([]repoStatusRow, 0, len(rowsByKey))
	for _, row := range rowsByKey {
		rows = append(rows, row)
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].Repo == rows[j].Repo {
			if rows[i].RepositoryOnly != rows[j].RepositoryOnly {
				return !rows[i].RepositoryOnly
			}
			return rows[i].EnvFile < rows[j].EnvFile
		}
		return rows[i].Repo < rows[j].Repo
	})
	return rows, nil
}

func applyStoredMetadata(row *repoStatusRow, metadata store.Metadata) {
	row.DriftStatus = "clean"
	row.BackupStatus = "backed_up"
	if metadata.LastBackupFingerprint != metadata.ContentFingerprint {
		row.BackupStatus = "backup_due"
	}
	row.ImportedAt = formatTimestamp(metadata.LastImportedAt)
	row.RemoteURL = metadata.RemoteURL
	row.BackupAt = "never"
	if metadata.LastBackupAt != nil {
		row.BackupAt = formatTimestamp(*metadata.LastBackupAt)
	}
}

func gitPresent(org config.Organization, repo string) bool {
	_, err := os.Stat(filepath.Join(org.RepoRoot, filepath.FromSlash(repo), ".git"))
	return err == nil
}

func statusRowKey(repo string, envFile string) string {
	return repo + "\x00" + envFile
}

func formatTimestamp(timestamp time.Time) string {
	if timestamp.IsZero() {
		return "never"
	}
	return timestamp.Format("2006-01-02T15:04:05Z07:00")
}
