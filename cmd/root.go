package cmd

import (
	"os"

	"github.com/spf13/cobra"
	"github.com/teliaz/dot-vault/internal/biometric"
	"github.com/teliaz/dot-vault/internal/config"
	"github.com/teliaz/dot-vault/internal/crypto"
	"github.com/teliaz/dot-vault/internal/orgs"
	"github.com/teliaz/dot-vault/internal/store"
)

type appContext struct {
	configManager *config.Manager
	orgService    *orgs.Service
	storeService  *store.Service
	authGate      *biometric.Gate
	keyProvider   *crypto.KeyProvider
}

var rootCmd = &cobra.Command{
	Use:   "dot-vault",
	Short: "Manage encrypted organization-level .env secrets",
}

func Execute() error {
	return rootCmd.Execute()
}

func init() {
	ctx := newAppContext()

	rootCmd.AddCommand(newOrgCommand(ctx))
	rootCmd.AddCommand(newRepoCommand(ctx))
	rootCmd.AddCommand(newStoreCommand(ctx))
	rootCmd.AddCommand(newTUICommand(ctx))
}

func newAppContext() *appContext {
	cfgManager := config.NewManager()
	keyProvider := crypto.NewKeyProvider(os.Getenv("DOT_VAULT_KEYRING_SERVICE"))
	orgService := orgs.NewService(cfgManager)

	return &appContext{
		configManager: cfgManager,
		orgService:    orgService,
		storeService:  store.NewService(cfgManager, keyProvider),
		authGate:      biometric.NewGate(keyProvider),
		keyProvider:   keyProvider,
	}
}
