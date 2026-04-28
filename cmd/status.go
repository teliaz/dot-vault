package cmd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/teliaz/dot-vault/internal/store"
)

type repoStatusRow struct {
	Repo         string
	EnvFile      string
	DriftStatus  string
	BackupStatus string
	ImportedAt   string
	BackupAt     string
	CurrentAt    string
	Missing      bool
}

func collectRepoStatusRows(ctx context.Context, app *appContext, orgName string, repoFilter string) ([]repoStatusRow, error) {
	repos, err := app.orgService.Scan(ctx, orgName)
	if err != nil {
		return nil, err
	}
	repos = filterRepositories(repos, repoFilter)
	if len(repos) == 0 {
		return nil, fmt.Errorf("no repositories matched")
	}

	var rows []repoStatusRow
	for _, repo := range repos {
		for _, envFile := range repo.EnvFiles {
			payload, err := os.ReadFile(envFile.AbsPath)
			if err != nil {
				return nil, fmt.Errorf("read %s: %w", envFile.AbsPath, err)
			}

			row := repoStatusRow{
				Repo:      repo.RelPath,
				EnvFile:   envFile.Name,
				CurrentAt: formatTimestamp(envFile.UpdatedAt),
			}

			metadata, err := app.storeService.Metadata(store.GetInput{
				Organization: orgName,
				Repository:   repo.RelPath,
				EnvFile:      envFile.Name,
			})
			if errors.Is(err, os.ErrNotExist) {
				row.Missing = true
				row.DriftStatus = "missing"
				row.BackupStatus = "none"
				rows = append(rows, row)
				continue
			}
			if err != nil {
				return nil, err
			}

			row.DriftStatus = "clean"
			if store.Fingerprint(payload) != metadata.ContentFingerprint {
				row.DriftStatus = "drift"
			}
			row.BackupStatus = "backed_up"
			if metadata.LastBackupFingerprint != metadata.ContentFingerprint {
				row.BackupStatus = "backup_due"
			}
			row.ImportedAt = formatTimestamp(metadata.LastImportedAt)
			row.BackupAt = "never"
			if metadata.LastBackupAt != nil {
				row.BackupAt = formatTimestamp(*metadata.LastBackupAt)
			}

			rows = append(rows, row)
		}
	}

	return rows, nil
}

func formatTimestamp(timestamp time.Time) string {
	if timestamp.IsZero() {
		return "never"
	}
	return timestamp.Format("2006-01-02T15:04:05Z07:00")
}
