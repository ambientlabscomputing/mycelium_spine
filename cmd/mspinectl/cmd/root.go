package cmd
package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var (
	// Global flags








































}	return nil	}		return fmt.Errorf("%s is required (use --%s flag or %s env var)", name, name, "MSPINE_"+name)	if value == "" {func checkRequired(name, value string) error {}	return defaultValue	}		return value	if value := os.Getenv(key); value != "" {func getEnvOrDefault(key, defaultValue string) string {}	rootCmd.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "Enable verbose output")	rootCmd.PersistentFlags().StringVar(&orgID, "org-id", getEnvOrDefault("MSPINE_ORG_ID", ""), "Organization ID")	rootCmd.PersistentFlags().StringVar(&serverID, "server-id", getEnvOrDefault("MSPINE_SERVER_ID", ""), "Server ID for authentication")	rootCmd.PersistentFlags().StringVar(&serverAddr, "server", getEnvOrDefault("MSPINE_SERVER", "localhost:9090"), "Mycelium Spine server address")	// Global flagsfunc init() {}	return rootCmd.Execute()func Execute() error {// Execute runs the root command}subscribing to mailboxes, and querying service state.`,Universal Messaging Service (UMS). It provides commands for publishing messages,	Long: `mspinectl is a command-line interface for interacting with Mycelium Spine	Short: "Mycelium Spine CLI - Control and interact with Mycelium Spine UMS",	Use:   "mspinectl",var rootCmd = &cobra.Command{)	verbose    bool	orgID      string	serverID   string	serverAddr string