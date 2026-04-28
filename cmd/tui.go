package cmd

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/spf13/cobra"
	"github.com/teliaz/dot-vault/internal/diff"
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

			selectedOrgName := orgName
			if selectedOrgName == "" {
				org, err := app.orgService.ResolveOrganization("")
				if err != nil {
					return err
				}
				selectedOrgName = org.Name
			}

			orgRows, err := collectTUIOrgs(app, selectedOrgName)
			if err != nil {
				return err
			}
			statusRows, err := collectRepoStatusRows(cmd.Context(), app, selectedOrgName, repoFilter)
			if err != nil {
				return err
			}
			if len(statusRows) == 0 {
				return fmt.Errorf("no env files found")
			}

			refresh := func(selected string) ([]tui.Row, error) {
				if strings.TrimSpace(selected) == "" {
					selected = selectedOrgName
				}
				statusRows, err := collectRepoStatusRows(cmd.Context(), app, selected, repoFilter)
				if err != nil {
					return nil, err
				}
				return toTUIRows(statusRows), nil
			}

			actions := tui.Actions{
				RefreshOrg: refresh,
				SelectOrg: func(selected string) ([]tui.Org, []tui.Row, string, error) {
					org, err := app.orgService.SetActive(cmd.Context(), selected)
					if err != nil {
						return nil, nil, "", err
					}
					selectedOrgName = org.Name
					orgRows, err := collectTUIOrgs(app, selectedOrgName)
					if err != nil {
						return nil, nil, "", err
					}
					rows, err := refresh(selectedOrgName)
					if err != nil {
						return nil, nil, "", err
					}
					return orgRows, rows, fmt.Sprintf("selected organization %s", org.Name), nil
				},
				UnlockOrg: func(selected string, passphrase string) ([]tui.Row, string, error) {
					org, err := app.orgService.ResolveOrganization(selected)
					if err != nil {
						return nil, "", err
					}
					if err := app.keyProvider.SetPassphrase(org.Name, passphrase); err != nil {
						return nil, "", err
					}
					if err := app.authGate.Authorize(cmd.Context(), org, "reveal"); err != nil {
						return nil, "", err
					}
					rows, err := refresh(org.Name)
					if err != nil {
						return nil, "", err
					}
					rows, err = enrichRowsWithDiffSummaries(cmd, app, org.Name, rows)
					if err != nil {
						return nil, "", err
					}
					return rows, fmt.Sprintf("unlocked %s and loaded comparisons", org.Name), nil
				},
				Import: func(row tui.Row) (string, error) {
					targetPath, err := tuiRowEnvPath(app, row.Organization, row)
					if err != nil {
						return "", err
					}
					payload, err := os.ReadFile(targetPath)
					if err != nil {
						return "", fmt.Errorf("read %s: %w", targetPath, err)
					}
					metadata, err := app.storeService.Put(cmd.Context(), store.PutInput{
						Organization: row.Organization,
						Repository:   row.Repo,
						EnvFile:      row.EnvFile,
						SourcePath:   targetPath,
						RemoteURL:    gitRemoteURL(filepath.Dir(targetPath)),
						Plaintext:    payload,
					})
					if err != nil {
						return "", err
					}
					return fmt.Sprintf("imported %s/%s", metadata.Repository, metadata.EnvFile), nil
				},
				Backup: func(row tui.Row) (string, error) {
					org, err := app.orgService.ResolveOrganization(row.Organization)
					if err != nil {
						return "", err
					}
					if err := app.authGate.Authorize(cmd.Context(), org, "backup"); err != nil {
						return "", err
					}

					result, err := app.storeService.Backup(cmd.Context(), store.BackupInput{
						Organization: row.Organization,
						Repository:   row.Repo,
						EnvFile:      row.EnvFile,
						RemoteURL:    rowRemoteURL(app, row.Organization, row),
					})
					if errors.Is(err, os.ErrNotExist) {
						targetPath, pathErr := tuiRowEnvPath(app, row.Organization, row)
						if pathErr != nil {
							return "", pathErr
						}
						payload, readErr := os.ReadFile(targetPath)
						if readErr != nil {
							return "", fmt.Errorf("read %s: %w", targetPath, readErr)
						}
						if _, putErr := app.storeService.Put(cmd.Context(), store.PutInput{
							Organization: row.Organization,
							Repository:   row.Repo,
							EnvFile:      row.EnvFile,
							SourcePath:   targetPath,
							RemoteURL:    gitRemoteURL(filepath.Dir(targetPath)),
							Plaintext:    payload,
						}); putErr != nil {
							return "", putErr
						}
						result, err = app.storeService.Backup(cmd.Context(), store.BackupInput{
							Organization: row.Organization,
							Repository:   row.Repo,
							EnvFile:      row.EnvFile,
							RemoteURL:    rowRemoteURL(app, row.Organization, row),
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
					org, err := app.orgService.ResolveOrganization(row.Organization)
					if err != nil {
						return "", err
					}
					if err := app.authGate.Authorize(cmd.Context(), org, "restore"); err != nil {
						return "", err
					}
					if !row.GitPresent {
						return "", fmt.Errorf("repository checkout is missing; press c to clone %s before restoring", row.Repo)
					}

					targetPath, err := tuiRowEnvPath(app, row.Organization, row)
					if err != nil {
						return "", err
					}
					plaintext, metadata, err := app.storeService.Get(cmd.Context(), store.GetInput{
						Organization: row.Organization,
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
				Clone: func(row tui.Row, cloneURL string) (string, error) {
					org, err := app.orgService.ResolveOrganization(row.Organization)
					if err != nil {
						return "", err
					}
					targetDir, err := safeRepoDirPath(org.RepoRoot, row.Repo)
					if err != nil {
						return "", err
					}
					if err := os.MkdirAll(filepath.Dir(targetDir), 0o755); err != nil {
						return "", fmt.Errorf("create clone parent directory: %w", err)
					}
					output, err := exec.CommandContext(cmd.Context(), "git", "clone", cloneURL, targetDir).CombinedOutput()
					if err != nil {
						return "", fmt.Errorf("git clone failed: %w\n%s", err, strings.TrimSpace(string(output)))
					}
					return fmt.Sprintf("cloned %s", row.Repo), nil
				},
				Diff: func(row tui.Row) (string, error) {
					return renderTUIDiff(cmd, app, row)
				},
			}

			return tui.RunDashboardWithDependencies(cmd.OutOrStdout(), orgRows, collectTUIDependencies(), toTUIRows(statusRows), actions)
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
			Organization: row.Organization,
			Repo:         row.Repo,
			EnvFile:      row.EnvFile,
			DriftStatus:  row.DriftStatus,
			BackupStatus: row.BackupStatus,
			ImportedAt:   row.ImportedAt,
			BackupAt:     row.BackupAt,
			CurrentAt:    row.CurrentAt,
			RemoteURL:    row.RemoteURL,
			GitPresent:   row.GitPresent,
			EnvPresent:   row.EnvPresent,
			StoreMissing: row.StoreMissing,
			DiffSummary:  row.DiffSummary,
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

func rowRemoteURL(app *appContext, orgName string, row tui.Row) string {
	if strings.TrimSpace(row.RemoteURL) != "" {
		return row.RemoteURL
	}
	org, err := app.orgService.ResolveOrganization(orgName)
	if err != nil {
		return ""
	}
	return gitRemoteURL(filepath.Join(org.RepoRoot, filepath.FromSlash(row.Repo)))
}

func collectTUIOrgs(app *appContext, selected string) ([]tui.Org, error) {
	cfg, err := app.configManager.Load()
	if err != nil {
		return nil, err
	}

	names := make([]string, 0, len(cfg.Organizations))
	for name := range cfg.Organizations {
		names = append(names, name)
	}
	sort.Strings(names)

	rows := make([]tui.Org, 0, len(names))
	for _, name := range names {
		org := cfg.Organizations[name]
		rows = append(rows, tui.Org{
			Name:      org.Name,
			Active:    org.Name == selected,
			RepoRoot:  org.RepoRoot,
			StoreRoot: org.StoreRoot,
		})
	}
	return rows, nil
}

func enrichRowsWithDiffSummaries(cmd *cobra.Command, app *appContext, orgName string, rows []tui.Row) ([]tui.Row, error) {
	enriched := make([]tui.Row, len(rows))
	copy(enriched, rows)

	for index, row := range enriched {
		summary, err := diffSummaryForRow(cmd, app, orgName, row)
		if err != nil {
			return nil, err
		}
		enriched[index].DiffSummary = summary
	}
	return enriched, nil
}

func diffSummaryForRow(cmd *cobra.Command, app *appContext, orgName string, row tui.Row) (string, error) {
	if row.StoreMissing {
		return "not stored", nil
	}
	if !row.EnvPresent {
		return "no repo env", nil
	}

	currentPayload, err := readTUIRowCurrentEnv(app, orgName, row)
	if err != nil {
		return "", err
	}
	storedPayload, _, err := app.storeService.Get(cmd.Context(), store.GetInput{
		Organization: orgName,
		Repository:   row.Repo,
		EnvFile:      row.EnvFile,
	})
	if err != nil {
		return "", err
	}

	result, err := compareEnvPayloads(storedPayload, currentPayload, row)
	if err != nil {
		return "", err
	}
	if !result.HasDrift() {
		return "clean", nil
	}
	return fmt.Sprintf("+%d -%d ~%d", result.AddedCount(), result.RemovedCount(), result.ChangedCount()), nil
}

func renderTUIDiff(cmd *cobra.Command, app *appContext, row tui.Row) (string, error) {
	orgName := row.Organization
	org, err := app.orgService.ResolveOrganization(orgName)
	if err != nil {
		return "", err
	}
	if err := app.authGate.Authorize(cmd.Context(), org, "reveal"); err != nil {
		return "", err
	}
	if row.StoreMissing {
		return "", fmt.Errorf("%s/%s is not stored yet", row.Repo, row.EnvFile)
	}
	if !row.EnvPresent {
		return "", fmt.Errorf("%s/%s is not present in the repository checkout", row.Repo, row.EnvFile)
	}

	currentPayload, err := readTUIRowCurrentEnv(app, orgName, row)
	if err != nil {
		return "", err
	}
	storedPayload, metadata, err := app.storeService.Get(cmd.Context(), store.GetInput{
		Organization: orgName,
		Repository:   row.Repo,
		EnvFile:      row.EnvFile,
	})
	if err != nil {
		return "", err
	}

	result, err := compareEnvPayloads(storedPayload, currentPayload, row)
	if err != nil {
		return "", err
	}
	var output strings.Builder
	status := "clean"
	if result.HasDrift() {
		status = "drift"
	}
	fmt.Fprintf(
		&output,
		"%s/%s: %s added=%d removed=%d changed=%d unchanged=%d imported=%s current=%s\n",
		row.Repo,
		row.EnvFile,
		status,
		result.AddedCount(),
		result.RemovedCount(),
		result.ChangedCount(),
		result.UnchangedCount(),
		metadata.LastImportedAt.Format("2006-01-02T15:04:05Z07:00"),
		row.CurrentAt,
	)
	for _, change := range result.Changes {
		if change.Type == diff.Unchanged {
			continue
		}
		switch change.Type {
		case diff.Added:
			fmt.Fprintf(&output, "+ %s=%s\n", change.Key, change.CurrentValue)
		case diff.Removed:
			fmt.Fprintf(&output, "- %s=%s\n", change.Key, change.StoredValue)
		case diff.Changed:
			fmt.Fprintf(&output, "~ %s: %s -> %s\n", change.Key, change.StoredValue, change.CurrentValue)
		}
	}
	if !result.HasDrift() {
		output.WriteString("No differences.\n")
	}
	return strings.TrimRight(output.String(), "\n"), nil
}

func readTUIRowCurrentEnv(app *appContext, orgName string, row tui.Row) ([]byte, error) {
	targetPath, err := tuiRowEnvPath(app, orgName, row)
	if err != nil {
		return nil, err
	}
	payload, err := os.ReadFile(targetPath)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", targetPath, err)
	}
	return payload, nil
}

func compareEnvPayloads(storedPayload []byte, currentPayload []byte, row tui.Row) (diff.Result, error) {
	storedEnv, err := diff.ParseEnv(storedPayload)
	if err != nil {
		return diff.Result{}, fmt.Errorf("parse stored %s/%s: %w", row.Repo, row.EnvFile, err)
	}
	currentEnv, err := diff.ParseEnv(currentPayload)
	if err != nil {
		return diff.Result{}, fmt.Errorf("parse current %s/%s: %w", row.Repo, row.EnvFile, err)
	}
	return diff.Compare(storedEnv, currentEnv), nil
}
