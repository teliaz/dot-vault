package cmd

import (
	"fmt"
	"sort"

	"github.com/spf13/cobra"
)

func newOrgCommand(app *appContext) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "org",
		Short: "Manage organizations",
	}

	cmd.AddCommand(newOrgAddCommand(app))
	cmd.AddCommand(newOrgListCommand(app))
	cmd.AddCommand(newOrgUseCommand(app))

	return cmd
}

func newOrgAddCommand(app *appContext) *cobra.Command {
	var name string
	var repoRoot string
	var storeRoot string
	var setActive bool

	cmd := &cobra.Command{
		Use:   "add",
		Short: "Register a new organization",
		RunE: func(cmd *cobra.Command, args []string) error {
			org, err := app.orgService.Add(cmd.Context(), name, repoRoot, storeRoot, setActive)
			if err != nil {
				return err
			}

			fmt.Fprintf(cmd.OutOrStdout(), "organization %q added\n", org.Name)
			fmt.Fprintf(cmd.OutOrStdout(), "repo root: %s\n", org.RepoRoot)
			fmt.Fprintf(cmd.OutOrStdout(), "store root: %s\n", org.StoreRoot)
			fmt.Fprintf(cmd.OutOrStdout(), "master key backend: %s\n", org.MasterKeyBackend)
			return nil
		},
	}

	cmd.Flags().StringVar(&name, "name", "", "Organization name")
	cmd.Flags().StringVar(&repoRoot, "repo-root", "", "Path where the organization's repositories live")
	cmd.Flags().StringVar(&storeRoot, "store-root", "", "Output path for encrypted secret storage")
	cmd.Flags().BoolVar(&setActive, "active", true, "Set this organization as the active default")
	_ = cmd.MarkFlagRequired("name")
	_ = cmd.MarkFlagRequired("repo-root")
	_ = cmd.MarkFlagRequired("store-root")

	return cmd
}

func newOrgListCommand(app *appContext) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List configured organizations",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := app.configManager.Load()
			if err != nil {
				return err
			}

			if len(cfg.Organizations) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "no organizations configured")
				return nil
			}

			names := make([]string, 0, len(cfg.Organizations))
			for name := range cfg.Organizations {
				names = append(names, name)
			}
			sort.Strings(names)

			for _, name := range names {
				org := cfg.Organizations[name]
				activeMark := " "
				if cfg.ActiveOrganization == name {
					activeMark = "*"
				}
				fmt.Fprintf(
					cmd.OutOrStdout(),
					"%s %s\n  repo root: %s\n  store root: %s\n  session ttl: %dm\n",
					activeMark,
					org.Name,
					org.RepoRoot,
					org.StoreRoot,
					org.AuthPolicy.SessionTTLMinutes,
				)
			}
			return nil
		},
	}
}

func newOrgUseCommand(app *appContext) *cobra.Command {
	return &cobra.Command{
		Use:   "use <name>",
		Short: "Set the active default organization",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			org, err := app.orgService.SetActive(cmd.Context(), args[0])
			if err != nil {
				return err
			}

			fmt.Fprintf(cmd.OutOrStdout(), "active organization set to %q\n", org.Name)
			return nil
		},
	}
}
