// Package cmd contains the spine client commands (subscribe, publish, ack).
// These commands connect to the main Spine gRPC service, not the admin socket.
package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

// clientFlags holds the flags shared across all client commands.
type clientFlags struct {
	serverAddr string
	serverID   string
	orgID      string
	verbose    bool
}

func defaultClientFlags() clientFlags {
	return clientFlags{
		serverAddr: getEnvOrDefault("MSPINE_SERVER", "localhost:9090"),
		serverID:   getEnvOrDefault("MSPINE_SERVER_ID", ""),
		orgID:      getEnvOrDefault("MSPINE_ORG_ID", ""),
	}
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

// ClientCmds returns a parent cobra.Command containing subscribe, publish, and ack.
func ClientCmds() *cobra.Command {
	f := defaultClientFlags()
	parent := &cobra.Command{
		Use:   "client",
		Short: "Spine client commands (subscribe, publish, ack)",
	}
	parent.PersistentFlags().StringVar(&f.serverAddr, "server", f.serverAddr, "Mycelium Spine server address")
	parent.PersistentFlags().StringVar(&f.serverID, "server-id", f.serverID, "Server ID for authentication")
	parent.PersistentFlags().StringVar(&f.orgID, "org-id", f.orgID, "Organization ID")
	parent.PersistentFlags().BoolVarP(&f.verbose, "verbose", "v", false, "Enable verbose output")

	parent.AddCommand(
		newSubscribeCmd(&f),
		newPublishCmd(&f),
		newAckCmd(&f),
	)
	return parent
}

// SubscribeCmd returns the subscribe command as a top-level command for direct registration.
func SubscribeCmd() *cobra.Command {
	f := defaultClientFlags()
	c := newSubscribeCmd(&f)
	c.PersistentFlags().StringVar(&f.serverAddr, "server", f.serverAddr, "Mycelium Spine server address")
	c.PersistentFlags().StringVar(&f.serverID, "server-id", f.serverID, "Server ID for authentication")
	c.PersistentFlags().StringVar(&f.orgID, "org-id", f.orgID, "Organization ID")
	c.PersistentFlags().BoolVarP(&f.verbose, "verbose", "v", false, "Enable verbose output")
	return c
}

// PublishCmd returns the publish command as a top-level command for direct registration.
func PublishCmd() *cobra.Command {
	f := defaultClientFlags()
	c := newPublishCmd(&f)
	c.PersistentFlags().StringVar(&f.serverAddr, "server", f.serverAddr, "Mycelium Spine server address")
	c.PersistentFlags().StringVar(&f.serverID, "server-id", f.serverID, "Server ID for authentication")
	c.PersistentFlags().StringVar(&f.orgID, "org-id", f.orgID, "Organization ID")
	c.PersistentFlags().BoolVarP(&f.verbose, "verbose", "v", false, "Enable verbose output")
	return c
}

// AckCmd returns the ack command as a top-level command for direct registration.
func AckCmd() *cobra.Command {
	f := defaultClientFlags()
	c := newAckCmd(&f)
	c.PersistentFlags().StringVar(&f.serverAddr, "server", f.serverAddr, "Mycelium Spine server address")
	c.PersistentFlags().StringVar(&f.serverID, "server-id", f.serverID, "Server ID for authentication")
	c.PersistentFlags().StringVar(&f.orgID, "org-id", f.orgID, "Organization ID")
	c.PersistentFlags().BoolVarP(&f.verbose, "verbose", "v", false, "Enable verbose output")
	return c
}
