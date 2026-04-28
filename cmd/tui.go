package cmd

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	"github.com/teliaz/dot-vault/internal/store"
	"github.com/teliaz/dot-vault/internal/tui"
)

func newTUICommand(app *appContext) *cobra.Command {
	var orgName string
	var repoFilter string

	cmd := &cobra.Command{
		Use:   "tui",
		Short: "Open the interactive env secrets dashboard",
		RunE: func(cmd *cobra.Command, args []string) error {
			app.keyProvider.SetInteractiveFallback(false)
			if orgName == "" {
				createdOrgName, err := runFirstRunSetupIfNeeded(cmd, app)
				if err != nil {
					return err
				}
				orgName = createdOrgName
			}

			statusRows, err := collectRepoStatusRows(cmd.Context(), app, orgName, repoFilter)
			if err != nil {
				return err
			}
			if len(statusRows) == 0 {
				return fmt.Errorf("no env files found")
			}

			refresh := func() ([]tui.Row, error) {
				statusRows, err := collectRepoStatusRows(cmd.Context(), app, orgName, repoFilter)
				if err != nil {
					return nil, err
				}
				return toTUIRows(statusRows), nil
			}

			actions := tui.Actions{
				Refresh: refresh,
				Unlock: func(passphrase string) (string, error) {
					org, err := app.orgService.ResolveOrganization(orgName)
					if err != nil {
						return "", err
					}
					if err := app.keyProvider.SetPassphrase(org.Name, passphrase); err != nil {
						return "", err
					}
					if err := app.authGate.Authorize(cmd.Context(), org, "restore"); err != nil {
						return "", err
					}
					return fmt.Sprintf("unlocked %s for sensitive actions", org.Name), nil
				},
				Import: func(row tui.Row) (string, error) {
					targetPath, err := tuiRowEnvPath(app, orgName, row)
					if err != nil {
						return "", err
					}
					payload, err := os.ReadFile(targetPath)
					if err != nil {
						return "", fmt.Errorf("read %s: %w", targetPath, err)
					}
					metadata, err := app.storeService.Put(cmd.Context(), store.PutInput{
						Organization: orgName,
						Repository:   row.Repo,
						EnvFile:      row.EnvFile,
						SourcePath:   targetPath,
						Plaintext:    payload,
					})
					if err != nil {
						return "", err
					}
					return fmt.Sprintf("imported %s/%s", metadata.Repository, metadata.EnvFile), nil
				},
				Backup: func(row tui.Row) (string, error) {
					org, err := app.orgService.ResolveOrganization(orgName)
					if err != nil {
						return "", err
					}
					if err := app.authGate.Authorize(cmd.Context(), org, "backup"); err != nil {
						return "", err
					}

					result, err := app.storeService.Backup(cmd.Context(), store.BackupInput{
						Organization: orgName,
						Repository:   row.Repo,
						EnvFile:      row.EnvFile,
					})
					if errors.Is(err, os.ErrNotExist) {
						targetPath, pathErr := tuiRowEnvPath(app, orgName, row)
						if pathErr != nil {
							return "", pathErr
						}
						payload, readErr := os.ReadFile(targetPath)
						if readErr != nil {
							return "", fmt.Errorf("read %s: %w", targetPath, readErr)
						}
						if _, putErr := app.storeService.Put(cmd.Context(), store.PutInput{
							Organization: orgName,
							Repository:   row.Repo,
							EnvFile:      row.EnvFile,
							SourcePath:   targetPath,
							Plaintext:    payload,
						}); putErr != nil {
							return "", putErr
						}
						result, err = app.storeService.Backup(cmd.Context(), store.BackupInput{
							Organization: orgName,
							Repository:   row.Repo,
							EnvFile:      row.EnvFile,
						})
					}
					if err != nil {
						return "", err
					}
					if !result.Created {
						return fmt.Sprintf("backup skipped for %s/%s", row.Repo, row.EnvFile), nil
					}
					return fmt.Sprintf("backed up %s/%s", row.Repo, row.EnvFile), nil
				},
				Restore: func(row tui.Row) (string, error) {
					org, err := app.orgService.ResolveOrganization(orgName)
					if err != nil {
						return "", err
					}
					if err := app.authGate.Authorize(cmd.Context(), org, "restore"); err != nil {
						return "", err
					}

					targetPath, err := tuiRowEnvPath(app, orgName, row)
					if err != nil {
						return "", err
					}
					plaintext, metadata, err := app.storeService.Get(cmd.Context(), store.GetInput{
						Organization: orgName,
						Repository:   row.Repo,
						EnvFile:      row.EnvFile,
					})
					if err != nil {
						return "", err
					}
					if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
						return "", fmt.Errorf("create restore directory: %w", err)
					}
					if err := os.WriteFile(targetPath, plaintext, 0o600); err != nil {
						return "", fmt.Errorf("write restored env file: %w", err)
					}
					return fmt.Sprintf("restored %s/%s", metadata.Repository, metadata.EnvFile), nil
				},
			}

			return tui.Run(cmd.OutOrStdout(), toTUIRows(statusRows), actions)
		},
	}

	cmd.Flags().StringVar(&orgName, "org", "", "Organization name; defaults to the active organization")
	cmd.Flags().StringVar(&repoFilter, "repo", "", "Repository name or path to inspect")
	return cmd
}

func runFirstRunSetupIfNeeded(cmd *cobra.Command, app *appContext) (string, error) {
	cfg, err := app.configManager.Load()
	if err != nil {
		return "", err
	}
	if len(cfg.Organizations) > 0 {
		return "", nil
	}

	result, err := tui.RunSetup(cmd.OutOrStdout(), tui.SetupActions{
		Create: func(input tui.SetupInput) (string, error) {
			if err := app.keyProvider.SetPassphrase(input.Name, input.MasterPassphrase); err != nil {
				return "", err
			}
			org, err := app.orgService.Add(cmd.Context(), input.Name, input.RepoRoot, input.StoreRoot, true)
			if err != nil {
				return "", err
			}
			if _, err := app.keyProvider.GetOrCreateMasterKey(cmd.Context(), org.Name); err != nil {
				return "", err
			}
			if err := app.authGate.Authorize(cmd.Context(), org, "restore"); err != nil {
				return "", err
			}
			return fmt.Sprintf("created organization %s", org.Name), nil
		},
	})
	if err != nil {
		return "", err
	}
	if !result.Created {
		return "", errors.New("first-run setup cancelled")
	}
	return result.Input.Name, nil
}

func toTUIRows(statusRows []repoStatusRow) []tui.Row {
	rows := make([]tui.Row, 0, len(statusRows))
	for _, row := range statusRows {
		rows = append(rows, tui.Row{
			Repo:         row.Repo,
			EnvFile:      row.EnvFile,
			DriftStatus:  row.DriftStatus,
			BackupStatus: row.BackupStatus,
			ImportedAt:   row.ImportedAt,
			BackupAt:     row.BackupAt,
			CurrentAt:    row.CurrentAt,
			Missing:      row.Missing,
		})
	}
	return rows
}

func tuiRowEnvPath(app *appContext, orgName string, row tui.Row) (string, error) {
	org, err := app.orgService.ResolveOrganization(orgName)
	if err != nil {
		return "", err
	}
	return safeRepoEnvPath(org.RepoRoot, row.Repo, row.EnvFile)
}
