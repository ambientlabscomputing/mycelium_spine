package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var (
	// Global flags
	serverAddr string
	serverID   string
	orgID      string
	verbose    bool
)

var rootCmd = &cobra.Command{
	Use:   "mspinectl",
	Short: "Mycelium Spine CLI - Control and interact with Mycelium Spine UMS",
	Long: `mspinectl is a command-line interface for interacting with Mycelium Spine
Universal Messaging Service (UMS). It provides commands for publishing messages,
subscribing to mailboxes, and querying service state.`,
}

// Execute runs the root command
func Execute() error {
	return rootCmd.Execute()
}

func init() {
	// Global flags
	rootCmd.PersistentFlags().StringVar(&serverAddr, "server", getEnvOrDefault("MSPINE_SERVER", "localhost:9090"), "Mycelium Spine server address")
	rootCmd.PersistentFlags().StringVar(&serverID, "server-id", getEnvOrDefault("MSPINE_SERVER_ID", ""), "Server ID for authentication")
	rootCmd.PersistentFlags().StringVar(&orgID, "org-id", getEnvOrDefault("MSPINE_ORG_ID", ""), "Organization ID")
	rootCmd.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "Enable verbose output")
}

func getEnvOrDefault(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func checkRequired(name, value string) error {
	if value == "" {
		return fmt.Errorf("%s is required (use --%s flag or %s env var)", name, name, "MSPINE_"+name)
	}
	return nil
}
