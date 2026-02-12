package cmd

import (
	"context"
	"fmt"
	"time"

	"github.com/ambientlabscomputing/mycelium_spine/sdk"
	"github.com/spf13/cobra"
)

var (
	ackMailboxID string
	ackSeq       uint64
)

var ackCmd = &cobra.Command{
	Use:   "ack",
	Short: "Acknowledge a message",
	Long:  `Send a cumulative acknowledgment for a message in a mailbox.`,
	Example: `  # Acknowledge message at sequence 42
  mspinectl ack --mailbox-id MAILBOX_ID --seq 42`,
	RunE: runAck,
}

func init() {
	rootCmd.AddCommand(ackCmd)

	ackCmd.Flags().StringVar(&ackMailboxID, "mailbox-id", "", "Mailbox ID")
	ackCmd.Flags().Uint64Var(&ackSeq, "seq", 0, "Sequence number to acknowledge")

	ackCmd.MarkFlagRequired("mailbox-id")
	ackCmd.MarkFlagRequired("seq")
}

func runAck(cmd *cobra.Command, args []string) error {
	// Validate required flags
	if err := checkRequired("server-id", serverID); err != nil {
		return err
	}
	if err := checkRequired("org-id", orgID); err != nil {
		return err
	}

	// Create client
	client, err := sdk.NewClient(serverAddr, sdk.ClientConfig{
		ServerID:        serverID,
		OrgID:           orgID,
		ProtocolVersion: "1.0",
	})
	if err != nil {
		return fmt.Errorf("failed to create client: %w", err)
	}
	defer client.Close()

	// Connect
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	if err := client.Connect(ctx); err != nil {
		cancel()
		return fmt.Errorf("failed to connect: %w", err)
	}
	cancel()

	if verbose {
		fmt.Printf("Connected with session ID: %s\n", client.SessionID())
	}

	// Send acknowledgment
	if err := client.Ack(ackMailboxID, ackSeq); err != nil {
		return fmt.Errorf("failed to send ack: %w", err)
	}

	fmt.Printf("Acknowledged mailbox %s up to sequence %d\n", ackMailboxID, ackSeq)

	// Give server time to process
	time.Sleep(100 * time.Millisecond)

	return nil
}
