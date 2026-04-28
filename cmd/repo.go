package cmd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"github.com/teliaz/dot-vault/internal/diff"
	"github.com/teliaz/dot-vault/internal/orgs"
	"github.com/teliaz/dot-vault/internal/store"
)

func newRepoCommand(app *appContext) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "repo",
		Short: "Manage repository env files",
	}

	cmd.AddCommand(newRepoScanCommand(app))
	cmd.AddCommand(newRepoStatusCommand(app))
	cmd.AddCommand(newRepoImportCommand(app))
	cmd.AddCommand(newRepoCompareCommand(app))
	cmd.AddCommand(newRepoBackupCommand(app))
	cmd.AddCommand(newRepoRestoreCommand(app))

	return cmd
}

func newRepoScanCommand(app *appContext) *cobra.Command {
	var orgName string

	cmd := &cobra.Command{
		Use:   "scan",
		Short: "Scan configured repositories for env files",
		RunE: func(cmd *cobra.Command, args []string) error {
			repos, err := app.orgService.Scan(cmd.Context(), orgName)
			if err != nil {
				return err
			}
			if len(repos) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "no repositories found")
				return nil
			}

			for _, repo := range repos {
				fmt.Fprintf(cmd.OutOrStdout(), "%s\n", repo.RelPath)
				if len(repo.EnvFiles) == 0 {
					fmt.Fprintln(cmd.OutOrStdout(), "  no env files")
					continue
				}
				for _, envFile := range repo.EnvFiles {
					fmt.Fprintf(
						cmd.OutOrStdout(),
						"  %s  %d bytes  updated %s\n",
						envFile.RelPath,
						envFile.Size,
						envFile.UpdatedAt.Format("2006-01-02T15:04:05Z07:00"),
					)
				}
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&orgName, "org", "", "Organization name; defaults to the active organization")
	return cmd
}

func newRepoStatusCommand(app *appContext) *cobra.Command {
	var orgName string
	var repoFilter string

	cmd := &cobra.Command{
		Use:   "status",
		Short: "Show import, drift, and backup status for env files",
		RunE: func(cmd *cobra.Command, args []string) error {
			rows, err := collectRepoStatusRows(cmd.Context(), app, orgName, repoFilter)
			if err != nil {
				return err
			}
			if len(rows) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "no env files found")
				return nil
			}
			for _, row := range rows {
				if row.Missing {
					fmt.Fprintf(
						cmd.OutOrStdout(),
						"%s/%s: missing current=%s backup=none\n",
						row.Repo,
						row.EnvFile,
						row.CurrentAt,
					)
					continue
				}
				fmt.Fprintf(
					cmd.OutOrStdout(),
					"%s/%s: %s %s imported=%s backup=%s current=%s\n",
					row.Repo,
					row.EnvFile,
					row.DriftStatus,
					row.BackupStatus,
					row.ImportedAt,
					row.BackupAt,
					row.CurrentAt,
				)
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&orgName, "org", "", "Organization name; defaults to the active organization")
	cmd.Flags().StringVar(&repoFilter, "repo", "", "Repository name or path to inspect")
	return cmd
}

func newRepoImportCommand(app *appContext) *cobra.Command {
	var orgName string
	var repoFilter string

	cmd := &cobra.Command{
		Use:   "import",
		Short: "Import discovered env files into the encrypted store",
		RunE: func(cmd *cobra.Command, args []string) error {
			repos, err := app.orgService.Scan(cmd.Context(), orgName)
			if err != nil {
				return err
			}
			repos = filterRepositories(repos, repoFilter)
			if len(repos) == 0 {
				return fmt.Errorf("no repositories matched")
			}

			imported := 0
			for _, repo := range repos {
				for _, envFile := range repo.EnvFiles {
					payload, err := os.ReadFile(envFile.AbsPath)
					if err != nil {
						return fmt.Errorf("read %s: %w", envFile.AbsPath, err)
					}

					metadata, err := app.storeService.Put(cmd.Context(), store.PutInput{
						Organization: orgName,
						Repository:   repo.RelPath,
						EnvFile:      envFile.Name,
						SourcePath:   envFile.AbsPath,
						Plaintext:    payload,
					})
					if err != nil {
						return err
					}

					imported++
					fmt.Fprintf(
						cmd.OutOrStdout(),
						"imported %s/%s fingerprint=%s\n",
						metadata.Repository,
						metadata.EnvFile,
						metadata.ContentFingerprint,
					)
				}
			}

			if imported == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "no env files found to import")
				return nil
			}
			fmt.Fprintf(cmd.OutOrStdout(), "imported %d env file(s)\n", imported)
			return nil
		},
	}

	cmd.Flags().StringVar(&orgName, "org", "", "Organization name; defaults to the active organization")
	cmd.Flags().StringVar(&repoFilter, "repo", "", "Repository name or path to import")
	return cmd
}

func newRepoCompareCommand(app *appContext) *cobra.Command {
	var orgName string
	var repoFilter string

	cmd := &cobra.Command{
		Use:   "compare",
		Short: "Compare current env files with encrypted stored versions",
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := authorizeSensitiveAction(cmd.Context(), app, orgName, "reveal"); err != nil {
				return err
			}

			repos, err := app.orgService.Scan(cmd.Context(), orgName)
			if err != nil {
				return err
			}
			repos = filterRepositories(repos, repoFilter)
			if len(repos) == 0 {
				return fmt.Errorf("no repositories matched")
			}

			compared := 0
			for _, repo := range repos {
				for _, envFile := range repo.EnvFiles {
					compared++
					currentPayload, err := os.ReadFile(envFile.AbsPath)
					if err != nil {
						return fmt.Errorf("read %s: %w", envFile.AbsPath, err)
					}

					storedPayload, metadata, err := app.storeService.Get(cmd.Context(), store.GetInput{
						Organization: orgName,
						Repository:   repo.RelPath,
						EnvFile:      envFile.Name,
					})
					if errors.Is(err, os.ErrNotExist) {
						fmt.Fprintf(cmd.OutOrStdout(), "%s/%s: missing from store\n", repo.RelPath, envFile.Name)
						continue
					}
					if err != nil {
						return err
					}

					storedEnv, err := diff.ParseEnv(storedPayload)
					if err != nil {
						return fmt.Errorf("parse stored %s/%s: %w", repo.RelPath, envFile.Name, err)
					}
					currentEnv, err := diff.ParseEnv(currentPayload)
					if err != nil {
						return fmt.Errorf("parse current %s: %w", envFile.AbsPath, err)
					}

					result := diff.Compare(storedEnv, currentEnv)
					printCompareResult(cmd, repo, envFile, metadata, result)
				}
			}

			if compared == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "no env files found to compare")
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&orgName, "org", "", "Organization name; defaults to the active organization")
	cmd.Flags().StringVar(&repoFilter, "repo", "", "Repository name or path to compare")
	return cmd
}

func newRepoBackupCommand(app *appContext) *cobra.Command {
	var orgName string
	var repoFilter string

	cmd := &cobra.Command{
		Use:   "backup",
		Short: "Create encrypted backup snapshots for imported env files",
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := authorizeSensitiveAction(cmd.Context(), app, orgName, "backup"); err != nil {
				return err
			}

			repos, err := app.orgService.Scan(cmd.Context(), orgName)
			if err != nil {
				return err
			}
			repos = filterRepositories(repos, repoFilter)
			if len(repos) == 0 {
				return fmt.Errorf("no repositories matched")
			}

			created := 0
			skipped := 0
			missing := 0
			for _, repo := range repos {
				for _, envFile := range repo.EnvFiles {
					result, err := app.storeService.Backup(cmd.Context(), store.BackupInput{
						Organization: orgName,
						Repository:   repo.RelPath,
						EnvFile:      envFile.Name,
					})
					if errors.Is(err, os.ErrNotExist) {
						missing++
						fmt.Fprintf(cmd.OutOrStdout(), "missing %s/%s from store\n", repo.RelPath, envFile.Name)
						continue
					}
					if err != nil {
						return err
					}

					if result.Created {
						created++
						fmt.Fprintf(cmd.OutOrStdout(), "backed up %s/%s -> %s\n", repo.RelPath, envFile.Name, result.BackupPath)
						continue
					}
					skipped++
					lastBackup := "never"
					if result.Metadata.LastBackupAt != nil {
						lastBackup = result.Metadata.LastBackupAt.Format("2006-01-02T15:04:05Z07:00")
					}
					fmt.Fprintf(cmd.OutOrStdout(), "skipped %s/%s unchanged since %s\n", repo.RelPath, envFile.Name, lastBackup)
				}
			}

			fmt.Fprintf(cmd.OutOrStdout(), "backup summary: created=%d skipped=%d missing=%d\n", created, skipped, missing)
			return nil
		},
	}

	cmd.Flags().StringVar(&orgName, "org", "", "Organization name; defaults to the active organization")
	cmd.Flags().StringVar(&repoFilter, "repo", "", "Repository name or path to backup")
	return cmd
}

func newRepoRestoreCommand(app *appContext) *cobra.Command {
	var orgName string
	var repoPath string
	var envFile string

	cmd := &cobra.Command{
		Use:   "restore",
		Short: "Restore one stored env file directly into a repository",
		RunE: func(cmd *cobra.Command, args []string) error {
			org, err := app.orgService.ResolveOrganization(orgName)
			if err != nil {
				return err
			}
			if err := app.authGate.Authorize(cmd.Context(), org, "restore"); err != nil {
				return err
			}
			repoPath = filepath.Clean(strings.TrimSpace(repoPath))
			envFile = strings.TrimSpace(envFile)
			if repoPath == "" || repoPath == "." {
				return fmt.Errorf("repo is required")
			}
			if envFile == "" || strings.Contains(envFile, "/") {
				return fmt.Errorf("env-file must be a file name")
			}

			plaintext, metadata, err := app.storeService.Get(cmd.Context(), store.GetInput{
				Organization: orgName,
				Repository:   repoPath,
				EnvFile:      envFile,
			})
			if err != nil {
				return err
			}

			targetPath, err := safeRepoEnvPath(org.RepoRoot, repoPath, envFile)
			if err != nil {
				return err
			}
			if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
				return fmt.Errorf("create restore directory: %w", err)
			}
			if err := os.WriteFile(targetPath, plaintext, 0o600); err != nil {
				return fmt.Errorf("write restored env file: %w", err)
			}

			fmt.Fprintf(
				cmd.OutOrStdout(),
				"restored %s/%s to %s imported=%s\n",
				metadata.Repository,
				metadata.EnvFile,
				targetPath,
				metadata.LastImportedAt.Format("2006-01-02T15:04:05Z07:00"),
			)
			return nil
		},
	}

	cmd.Flags().StringVar(&orgName, "org", "", "Organization name; defaults to the active organization")
	cmd.Flags().StringVar(&repoPath, "repo", "", "Repository path relative to the organization's repo root")
	cmd.Flags().StringVar(&envFile, "env-file", "", "Env file name such as .env or .env.production")
	_ = cmd.MarkFlagRequired("repo")
	_ = cmd.MarkFlagRequired("env-file")
	return cmd
}

func authorizeSensitiveAction(ctx context.Context, app *appContext, orgName string, action string) error {
	org, err := app.orgService.ResolveOrganization(orgName)
	if err != nil {
		return err
	}
	return app.authGate.Authorize(ctx, org, action)
}

func filterRepositories(repos []orgs.Repository, filter string) []orgs.Repository {
	filter = strings.TrimSpace(filter)
	if filter == "" {
		return repos
	}

	var matched []orgs.Repository
	for _, repo := range repos {
		if repo.Name == filter || repo.RelPath == filter {
			matched = append(matched, repo)
		}
	}
	return matched
}

func safeRepoEnvPath(repoRoot string, repoPath string, envFile string) (string, error) {
	cleanRepoPath := filepath.Clean(repoPath)
	if cleanRepoPath == "." || cleanRepoPath == ".." || strings.HasPrefix(cleanRepoPath, "../") || filepath.IsAbs(cleanRepoPath) {
		return "", fmt.Errorf("repo must be relative to the organization repo root")
	}
	if envFile == "" || filepath.Base(envFile) != envFile {
		return "", fmt.Errorf("env-file must be a file name")
	}

	targetPath := filepath.Join(repoRoot, cleanRepoPath, envFile)
	relPath, err := filepath.Rel(repoRoot, targetPath)
	if err != nil {
		return "", fmt.Errorf("resolve restore path: %w", err)
	}
	if relPath == "." || relPath == ".." || strings.HasPrefix(relPath, "../") || filepath.IsAbs(relPath) {
		return "", fmt.Errorf("restore target escapes organization repo root")
	}
	return targetPath, nil
}

func printCompareResult(cmd *cobra.Command, repo orgs.Repository, envFile orgs.EnvFile, metadata store.Metadata, result diff.Result) {
	status := "clean"
	if result.HasDrift() {
		status = "drift"
	}

	fmt.Fprintf(
		cmd.OutOrStdout(),
		"%s/%s: %s added=%d removed=%d changed=%d unchanged=%d imported=%s current=%s\n",
		repo.RelPath,
		envFile.Name,
		status,
		result.AddedCount(),
		result.RemovedCount(),
		result.ChangedCount(),
		result.UnchangedCount(),
		metadata.LastImportedAt.Format("2006-01-02T15:04:05Z07:00"),
		envFile.UpdatedAt.Format("2006-01-02T15:04:05Z07:00"),
	)
	for _, change := range result.Changes {
		if change.Type == diff.Unchanged {
			continue
		}
		switch change.Type {
		case diff.Added:
			fmt.Fprintf(cmd.OutOrStdout(), "  + %s=%s\n", change.Key, change.CurrentValue)
		case diff.Removed:
			fmt.Fprintf(cmd.OutOrStdout(), "  - %s=%s\n", change.Key, change.StoredValue)
		case diff.Changed:
			fmt.Fprintf(cmd.OutOrStdout(), "  ~ %s: %s -> %s\n", change.Key, change.StoredValue, change.CurrentValue)
		}
	}
}
