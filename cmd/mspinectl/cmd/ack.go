package cmd

import (
	"context"
	"fmt"
	"time"

	"github.com/ambientlabscomputing/mycelium_spine/sdk"
	"github.com/spf13/cobra"
)

func newAckCmd(f *clientFlags) *cobra.Command {
	var (
		mailboxID string
		seq       uint64
	)

	cmd := &cobra.Command{
		Use:   "ack",
		Short: "Acknowledge a message",
		Long:  `Send a cumulative acknowledgment for a message in a mailbox.`,
		Example: `  # Acknowledge message at sequence 42
  mspinectl ack --mailbox-id MAILBOX_ID --seq 42`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := checkRequired("server-id", f.serverID); err != nil {
				return err
			}
			if err := checkRequired("org-id", f.orgID); err != nil {
				return err
			}

			client, err := sdk.NewClient(f.serverAddr, sdk.ClientConfig{
				ServerID:        f.serverID,
				OrgID:           f.orgID,
				ProtocolVersion: "1.0",
			})
			if err != nil {
				return fmt.Errorf("failed to create client: %w", err)
			}
			defer client.Close()

			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			if err := client.Connect(ctx); err != nil {
				cancel()
				return fmt.Errorf("failed to connect: %w", err)
			}
			cancel()

			if f.verbose {
				fmt.Printf("Connected with session ID: %s\n", client.SessionID())
			}

			if err := client.Ack(mailboxID, seq); err != nil {
				return fmt.Errorf("failed to send ack: %w", err)
			}

			fmt.Printf("Acknowledged mailbox %s up to sequence %d\n", mailboxID, seq)

			// Give server time to process
			time.Sleep(100 * time.Millisecond)

			return nil
		},
	}

	cmd.Flags().StringVar(&mailboxID, "mailbox-id", "", "Mailbox ID")
	cmd.Flags().Uint64Var(&seq, "seq", 0, "Sequence number to acknowledge")
	_ = cmd.MarkFlagRequired("mailbox-id")
	_ = cmd.MarkFlagRequired("seq")

	return cmd
}
