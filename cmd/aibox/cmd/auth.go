package cmd

import (
	"context"
	"fmt"
	"time"

	"github.com/aibox/aibox/internal/credentials"
	"github.com/spf13/cobra"
)

var authCmd = &cobra.Command{
	Use:   "auth",
	Short: "Manage sandbox credentials",
	Long: `Auth provides subcommands for storing and managing credentials
used by AI-Box sandbox containers. Credentials are stored in the OS
keychain when available, or in an encrypted file as a fallback.`,
}

var authAddCmd = &cobra.Command{
	Use:   "add <type> <value>",
	Short: "Store a credential",
	Long: `Store a credential for use in sandbox containers.

Supported credential types:
  git-token      Git authentication token
  llm-api-key    LLM provider API key
  mirror-token   Package mirror authentication token`,
	Args: cobra.ExactArgs(2),
	RunE: runAuthAdd,
}

var authListCmd = &cobra.Command{
	Use:   "list",
	Short: "List stored credentials",
	Long:  `List all known credential types and their storage status.`,
	RunE:  runAuthList,
}

var authRemoveCmd = &cobra.Command{
	Use:   "remove <type>",
	Short: "Remove a stored credential",
	Long: `Remove a stored credential by type.

Supported credential types:
  git-token      Git authentication token
  llm-api-key    LLM provider API key
  mirror-token   Package mirror authentication token`,
	Args: cobra.ExactArgs(1),
	RunE: runAuthRemove,
}

func init() {
	authCmd.AddCommand(authAddCmd)
	authCmd.AddCommand(authListCmd)
	authCmd.AddCommand(authRemoveCmd)
	rootCmd.AddCommand(authCmd)
}

func newProvider() (credentials.Provider, error) {
	return credentials.NewKeychainProvider()
}

func runAuthAdd(cmd *cobra.Command, args []string) error {
	credType := args[0]
	credValue := args[1]

	if !credentials.ValidCredentialType(credType) {
		return fmt.Errorf("unknown credential type %q (valid: git-token, llm-api-key, mirror-token)", credType)
	}

	provider, err := newProvider()
	if err != nil {
		return fmt.Errorf("initializing credential provider: %w", err)
	}

	ctx := context.Background()
	cred := &credentials.Credential{
		Type:   credentials.CredentialType(credType),
		Value:  credValue,
		Source: provider.Name(),
	}

	if err := provider.Store(ctx, cred); err != nil {
		return fmt.Errorf("storing credential: %w", err)
	}

	fmt.Printf("Stored %s in OS keychain (%s).\n", credType, provider.Name())
	return nil
}

func runAuthList(cmd *cobra.Command, args []string) error {
	provider, err := newProvider()
	if err != nil {
		return fmt.Errorf("initializing credential provider: %w", err)
	}

	ctx := context.Background()
	broker := credentials.NewBroker(provider)
	statuses := broker.ValidateCredentials(ctx)

	fmt.Println("Stored credentials:")
	for _, s := range statuses {
		if s.Present {
			detail := fmt.Sprintf("source: %s", s.Source)
			if s.Expired {
				detail += ", EXPIRED"
			} else if s.ExpiresIn > 0 {
				detail += fmt.Sprintf(", expires in %s", s.ExpiresIn.Truncate(time.Minute))
			}
			fmt.Printf("  %-14s [ok] (%s)\n", s.Type, detail)
		} else {
			fmt.Printf("  %-14s [--] not configured\n", s.Type)
		}
	}

	fmt.Println()
	fmt.Println("Use 'aibox auth add <type> <value>' to configure missing credentials.")
	return nil
}

func runAuthRemove(cmd *cobra.Command, args []string) error {
	credType := args[0]

	if !credentials.ValidCredentialType(credType) {
		return fmt.Errorf("unknown credential type %q (valid: git-token, llm-api-key, mirror-token)", credType)
	}

	provider, err := newProvider()
	if err != nil {
		return fmt.Errorf("initializing credential provider: %w", err)
	}

	ctx := context.Background()
	if err := provider.Delete(ctx, credentials.CredentialType(credType)); err != nil {
		return fmt.Errorf("removing credential: %w", err)
	}

	fmt.Printf("Removed %s from OS keychain (%s).\n", credType, provider.Name())
	return nil
}
