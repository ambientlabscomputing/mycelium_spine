package cmd

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/ambientlabscomputing/mycelium_spine/sdk"
	umsv1 "github.com/ambientlabscomputing/mycelium_spine/proto/ums/v1"
	"github.com/spf13/cobra"
)

var (
	subscribeTargetType string
	subscribeTargetID   string
	subscribeCount      int
	subscribeAutoAck    bool
)

var subscribeCmd = &cobra.Command{
	Use:   "subscribe",
	Short: "Subscribe to a mailbox and receive messages",
	Long: `Subscribe to a target mailbox and receive delivered envelopes.
Messages will be printed to stdout as they arrive.`,
	Example: `  # Subscribe to server mailbox
  mspinectl subscribe --target-type server --target-id my-server-01

  # Subscribe and auto-acknowledge
  mspinectl subscribe --target-type server --target-id my-server-01 --auto-ack

  # Subscribe to cluster, receive 10 messages
  mspinectl subscribe --target-type cluster --target-id cluster-west --count 10`,
	RunE: runSubscribe,
}

func init() {
	rootCmd.AddCommand(subscribeCmd)

	subscribeCmd.Flags().StringVar(&subscribeTargetType, "target-type", "", "Target type: server, cluster, org, service")
	subscribeCmd.Flags().StringVar(&subscribeTargetID, "target-id", "", "Target ID")
	subscribeCmd.Flags().IntVar(&subscribeCount, "count", 0, "Number of messages to receive (0 = infinite)")
	subscribeCmd.Flags().BoolVar(&subscribeAutoAck, "auto-ack", false, "Automatically acknowledge messages")

	subscribeCmd.MarkFlagRequired("target-type")
	subscribeCmd.MarkFlagRequired("target-id")
}

func runSubscribe(cmd *cobra.Command, args []string) error {
	// Validate required flags
	if err := checkRequired("server-id", serverID); err != nil {
		return err
	}
	if err := checkRequired("org-id", orgID); err != nil {
		return err
	}

	// Parse target type
	targetType, err := parseTargetType(subscribeTargetType)
	if err != nil {
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

	// Subscribe
	target := &umsv1.Target{
		TargetType: targetType,
		TargetId:   subscribeTargetID,
		OrgId:      orgID,
	}

	if err := client.Subscribe([]*umsv1.Target{target}); err != nil {
		return fmt.Errorf("failed to subscribe: %w", err)
	}

	fmt.Printf("Subscribed to %s: %s\n", subscribeTargetType, subscribeTargetID)

	// Handle signals for graceful shutdown
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)

	messageCount := 0

	// Receive messages
	for {
		select {
		case delivery := <-client.Deliveries():
			for _, envelope := range delivery.Envelopes {
				messageCount++

				// Print message
				fmt.Printf("\n[%s] Message %d:\n", time.Now().Format(time.RFC3339), messageCount)
				fmt.Printf("  Mailbox ID:  %s\n", envelope.MailboxId)
				fmt.Printf("  Sequence:    %d\n", envelope.Seq)
				fmt.Printf("  Type:        %s\n", envelope.Type)
				fmt.Printf("  QoS:         %s\n", envelope.Qos)
				fmt.Printf("  Payload:     %s\n", string(envelope.Payload))

				// Auto-acknowledge
				if subscribeAutoAck {
					if err := client.Ack(envelope.MailboxId, envelope.Seq); err != nil {
						fmt.Fprintf(os.Stderr, "Failed to ack: %v\n", err)
					} else if verbose {
						fmt.Printf("  Acknowledged\n")
					}
				}

				// Check count limit
				if subscribeCount > 0 && messageCount >= subscribeCount {
					fmt.Printf("\nReceived %d messages, exiting\n", messageCount)
					return nil
				}
			}

		case err := <-client.Errors():
			return fmt.Errorf("client error: %w", err)

		case <-sigCh:
			fmt.Printf("\nReceived interrupt, shutting down...\n")
			return nil
		}
	}
}
