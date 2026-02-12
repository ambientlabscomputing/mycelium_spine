package cmd
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






































































































































}	}		return "UNSPECIFIED"	default:		return "TELEMETRY"	case umsv1.QoS_QOS_TELEMETRY:		return "CONTROL"	case umsv1.QoS_QOS_CONTROL:		return "COMMAND"	case umsv1.QoS_QOS_COMMAND:	switch qos {func formatQoS(qos umsv1.QoS) string {}	}		}			return nil			fmt.Printf("\n✓ Received %d messages\n", messageCount)		case <-sigCh:			return err			fmt.Fprintf(os.Stderr, "Client error: %v\n", err)		case err := <-client.Errors():			}				}					return nil					fmt.Printf("\n✓ Received %d messages, exiting\n", messageCount)				if subscribeCount > 0 && messageCount >= subscribeCount {				// Check count limit				}					}						fmt.Printf("  ✓ Acknowledged\n")					} else if verbose {						fmt.Fprintf(os.Stderr, "Failed to ack: %v\n", err)					if err := client.Ack(envelope.MailboxId, envelope.Seq); err != nil {				if subscribeAutoAck {				// Auto-ack if enabled				fmt.Println()				fmt.Printf("  Payload: %s\n", string(envelope.Payload))				fmt.Printf("  QoS:     %s\n", formatQoS(envelope.Qos))				fmt.Printf("  Type:    %s\n", envelope.Type)				fmt.Printf("  Seq:     %d\n", envelope.Seq)				fmt.Printf("  Mailbox: %s\n", envelope.MailboxId)				fmt.Printf("[%s] Message %d\n", time.Now().Format("15:04:05"), messageCount)				messageCount++			for _, envelope := range delivery.Envelopes {			}				return fmt.Errorf("delivery channel closed")			if delivery == nil {		case delivery := <-client.Deliveries():		select {	for {	// Receive loop	messageCount := 0	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)	sigCh := make(chan os.Signal, 1)	// Setup signal handling	fmt.Println()	fmt.Println("Waiting for messages... (Ctrl+C to exit)")	fmt.Printf("✓ Subscribed to %s:%s\n", subscribeTargetType, subscribeTargetID)	}		return fmt.Errorf("failed to subscribe: %w", err)	if err := client.Subscribe(targets); err != nil {	}		},			OrgId:      orgID,			TargetId:   subscribeTargetID,			TargetType: targetType,		{	targets := []*umsv1.Target{	// Subscribe	fmt.Printf("✓ Connected (session: %s)\n", client.SessionID())	}		return fmt.Errorf("failed to connect: %w", err)	if err := client.Connect(ctx); err != nil {	ctx := context.Background()	// Connect	defer client.Close()	}		return fmt.Errorf("failed to create client: %w", err)	if err != nil {	})		ProtocolVersion: "1.0",		OrgID:           orgID,		ServerID:        serverID,	client, err := sdk.NewClient(serverAddr, sdk.ClientConfig{	// Create client	}		return err	if err != nil {	targetType, err := parseTargetType(subscribeTargetType)	// Parse target type	}		return err	if err := checkRequired("org-id", orgID); err != nil {	}		return err	if err := checkRequired("server-id", serverID); err != nil {	// Validate required flagsfunc runSubscribe(cmd *cobra.Command, args []string) error {}	subscribeCmd.MarkFlagRequired("target-id")	subscribeCmd.MarkFlagRequired("target-type")	subscribeCmd.Flags().BoolVar(&subscribeAutoAck, "auto-ack", false, "Automatically acknowledge messages")	subscribeCmd.Flags().IntVarP(&subscribeCount, "count", "n", 0, "Exit after receiving n messages (0 = infinite)")	subscribeCmd.Flags().StringVar(&subscribeTargetID, "target-id", "", "Target ID")	subscribeCmd.Flags().StringVar(&subscribeTargetType, "target-type", "", "Target type: server, cluster, org, service, broadcast")	rootCmd.AddCommand(subscribeCmd)func init() {}	RunE: runSubscribe,  mspinectl subscribe --target-type cluster --target-id cluster-west --count 5`,  # Receive only 5 messages then exit  mspinectl subscribe --target-type server --target-id my-server-01 --auto-ack