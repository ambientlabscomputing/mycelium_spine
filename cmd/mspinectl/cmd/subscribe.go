package cmd

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	umsv1 "github.com/ambientlabscomputing/mycelium_spine/proto/ums/v1"
	"github.com/ambientlabscomputing/mycelium_spine/sdk"
	"github.com/spf13/cobra"
)

func newSubscribeCmd(f *clientFlags) *cobra.Command {
	var (
		targetType string
		targetID   string
		count      int
		autoAck    bool
	)

	cmd := &cobra.Command{
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
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := checkRequired("server-id", f.serverID); err != nil {
				return err
			}
			if err := checkRequired("org-id", f.orgID); err != nil {
				return err
			}

			tt, err := parseUMSTargetType(targetType)
			if err != nil {
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

			target := &umsv1.Target{
				TargetType: tt,
				TargetId:   targetID,
				OrgId:      f.orgID,
			}

			if err := client.Subscribe([]*umsv1.Target{target}); err != nil {
				return fmt.Errorf("failed to subscribe: %w", err)
			}

			fmt.Printf("Subscribed to %s: %s\n", targetType, targetID)

			sigCh := make(chan os.Signal, 1)
			signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)

			messageCount := 0
			for {
				select {
				case delivery := <-client.Deliveries():
					for _, envelope := range delivery.Envelopes {
						messageCount++
						fmt.Printf("\n[%s] Message %d:\n", time.Now().Format(time.RFC3339), messageCount)
						fmt.Printf("  Mailbox ID:  %s\n", envelope.MailboxId)
						fmt.Printf("  Sequence:    %d\n", envelope.Seq)
						fmt.Printf("  Type:        %s\n", envelope.Type)
						fmt.Printf("  QoS:         %s\n", envelope.Qos)
						fmt.Printf("  Payload:     %s\n", string(envelope.Payload))

						if autoAck {
							if err := client.Ack(envelope.MailboxId, envelope.Seq); err != nil {
								fmt.Fprintf(os.Stderr, "Failed to ack: %v\n", err)
							} else if f.verbose {
								fmt.Printf("  Acknowledged\n")
							}
						}

						if count > 0 && messageCount >= count {
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
		},
	}

	cmd.Flags().StringVar(&targetType, "target-type", "", "Target type: server, cluster, org, service")
	cmd.Flags().StringVar(&targetID, "target-id", "", "Target ID")
	cmd.Flags().IntVar(&count, "count", 0, "Number of messages to receive (0 = infinite)")
	cmd.Flags().BoolVar(&autoAck, "auto-ack", false, "Automatically acknowledge messages")
	_ = cmd.MarkFlagRequired("target-type")
	_ = cmd.MarkFlagRequired("target-id")

	return cmd
}

// parseUMSTargetType converts the string target type to a proto enum.
func parseUMSTargetType(s string) (umsv1.TargetType, error) {
	switch strings.ToUpper(s) {
	case "SERVER":
		return umsv1.TargetType_TARGET_TYPE_SERVER, nil
	case "CLUSTER":
		return umsv1.TargetType_TARGET_TYPE_CLUSTER, nil
	case "ORG":
		return umsv1.TargetType_TARGET_TYPE_ORG, nil
	case "SERVICE":
		return umsv1.TargetType_TARGET_TYPE_SERVICE, nil
	case "BROADCAST":
		return umsv1.TargetType_TARGET_TYPE_BROADCAST, nil
	default:
		return umsv1.TargetType_TARGET_TYPE_UNSPECIFIED, fmt.Errorf("unknown target type %q", s)
	}
}
