package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"github.com/teliaz/dot-vault/internal/store"
)

func newStoreCommand(app *appContext) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "store",
		Short: "Read and write encrypted secret files",
	}

	cmd.AddCommand(newStorePutCommand(app))
	cmd.AddCommand(newStoreGetCommand(app))

	return cmd
}

func newStorePutCommand(app *appContext) *cobra.Command {
	var orgName string
	var repo string
	var envFile string
	var sourcePath string
	var fromFile string
	var remoteURL string

	cmd := &cobra.Command{
		Use:   "put",
		Short: "Encrypt and store one env file",
		RunE: func(cmd *cobra.Command, args []string) error {
			plaintext, err := os.ReadFile(fromFile)
			if err != nil {
				return fmt.Errorf("read source file: %w", err)
			}

			metadata, err := app.storeService.Put(cmd.Context(), store.PutInput{
				Organization: orgName,
				Repository:   repo,
				EnvFile:      envFile,
				SourcePath:   sourcePath,
				RemoteURL:    remoteURL,
				Plaintext:    plaintext,
			})
			if err != nil {
				return err
			}

			fmt.Fprintf(cmd.OutOrStdout(), "stored %s/%s\n", metadata.Repository, metadata.EnvFile)
			if strings.TrimSpace(metadata.RemoteURL) != "" {
				fmt.Fprintf(cmd.OutOrStdout(), "remote: %s\n", metadata.RemoteURL)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "fingerprint: %s\n", metadata.ContentFingerprint)
			fmt.Fprintf(cmd.OutOrStdout(), "imported at: %s\n", metadata.LastImportedAt.Format("2006-01-02T15:04:05Z07:00"))
			return nil
		},
	}

	cmd.Flags().StringVar(&orgName, "org", "", "Organization name; defaults to the active organization")
	cmd.Flags().StringVar(&repo, "repo", "", "Repository path relative to the organization's repo root")
	cmd.Flags().StringVar(&envFile, "env-file", "", "Env file name such as .env or .env.production")
	cmd.Flags().StringVar(&sourcePath, "source-path", "", "Original repo path of the source env file")
	cmd.Flags().StringVar(&fromFile, "from-file", "", "Plaintext file to encrypt and import")
	cmd.Flags().StringVar(&remoteURL, "remote-url", "", "Optional Git remote URL for future clone/restore workflows")
	_ = cmd.MarkFlagRequired("repo")
	_ = cmd.MarkFlagRequired("env-file")
	_ = cmd.MarkFlagRequired("source-path")
	_ = cmd.MarkFlagRequired("from-file")

	return cmd
}

func newStoreGetCommand(app *appContext) *cobra.Command {
	var orgName string
	var repo string
	var envFile string
	var outPath string

	cmd := &cobra.Command{
		Use:   "get",
		Short: "Decrypt a stored env file",
		RunE: func(cmd *cobra.Command, args []string) error {
			action := "reveal"
			if outPath != "" {
				action = "restore"
			}
			if err := authorizeSensitiveAction(cmd.Context(), app, orgName, action); err != nil {
				return err
			}

			plaintext, metadata, err := app.storeService.Get(cmd.Context(), store.GetInput{
				Organization: orgName,
				Repository:   repo,
				EnvFile:      envFile,
			})
			if err != nil {
				return err
			}

			if outPath != "" {
				if err := os.WriteFile(outPath, plaintext, 0o600); err != nil {
					return fmt.Errorf("write output file: %w", err)
				}
				fmt.Fprintf(cmd.OutOrStdout(), "restored %s/%s to %s\n", metadata.Repository, metadata.EnvFile, outPath)
				return nil
			}

			if _, err := cmd.OutOrStdout().Write(plaintext); err != nil {
				return fmt.Errorf("write plaintext output: %w", err)
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&orgName, "org", "", "Organization name; defaults to the active organization")
	cmd.Flags().StringVar(&repo, "repo", "", "Repository path relative to the organization's repo root")
	cmd.Flags().StringVar(&envFile, "env-file", "", "Env file name such as .env or .env.production")
	cmd.Flags().StringVar(&outPath, "out", "", "Optional file path to write the decrypted contents to")
	_ = cmd.MarkFlagRequired("repo")
	_ = cmd.MarkFlagRequired("env-file")

	return cmd
}
