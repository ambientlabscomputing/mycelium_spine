package cmd
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

























}	return nil	time.Sleep(100 * time.Millisecond)	// Give time for ack to be sent	fmt.Printf("✓ Acknowledged mailbox %s up to seq %d\n", ackMailboxID, ackSeq)	}		return fmt.Errorf("failed to send ack: %w", err)	if err := client.Ack(ackMailboxID, ackSeq); err != nil {	// Send ack	}		fmt.Printf("Connected (session: %s)\n", client.SessionID())	if verbose {	}		return fmt.Errorf("failed to connect: %w", err)	if err := client.Connect(ctx); err != nil {	defer cancel()	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)	// Connect