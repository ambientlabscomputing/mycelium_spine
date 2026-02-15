package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	umsv1 "github.com/ambientlabscomputing/mycelium_spine/proto/ums/v1"
	"github.com/ambientlabscomputing/mycelium_spine/sdk"
	"github.com/spf13/cobra"
)

var (
	publishType       string
	publishQoS        string
	publishData       string
	publishFile       string
	publishTargetType string
	publishTargetID   string
)

var publishCmd = &cobra.Command{
	Use:   "publish",
	Short: "Publish a message to targets",
	Long:  `Publish an envelope (message) to one or more targets.`,
	Example: `  # Publish a command to a server
  mspinectl publish --type command.deploy --qos command \
    --target-type server --target-id prod-01 \
    --data '{"version":"v2.0"}'

  # Publish from file
  mspinectl publish --type command.config --qos command \
    --target-type cluster --target-id cluster-west \
    --file ./config.json`,
	RunE: runPublish,
}

func init() {
	rootCmd.AddCommand(publishCmd)

	publishCmd.Flags().StringVar(&publishType, "type", "", "Message type (e.g., command.deploy)")
	publishCmd.Flags().StringVar(&publishQoS, "qos", "control", "QoS level: command, control, or telemetry")
	publishCmd.Flags().StringVar(&publishData, "data", "", "JSON payload data")
	publishCmd.Flags().StringVar(&publishFile, "file", "", "File containing JSON payload")
	publishCmd.Flags().StringVar(&publishTargetType, "target-type", "", "Target type: server, cluster, org, service, or broadcast")
	publishCmd.Flags().StringVar(&publishTargetID, "target-id", "", "Target ID")

	publishCmd.MarkFlagRequired("type")
	publishCmd.MarkFlagRequired("target-type")
}

func runPublish(cmd *cobra.Command, args []string) error {
	// Validate required flags
	if err := checkRequired("server-id", serverID); err != nil {
		return err
	}
	if err := checkRequired("org-id", orgID); err != nil {
		return err
	}

	// Get payload
	var payload []byte
	if publishData != "" {
		payload = []byte(publishData)
	} else if publishFile != "" {
		data, err := os.ReadFile(publishFile)
		if err != nil {
			return fmt.Errorf("failed to read file: %w", err)
		}
		payload = data
	} else {
		return fmt.Errorf("either --data or --file must be provided")
	}

	// Validate JSON
	if !json.Valid(payload) {
		return fmt.Errorf("payload is not valid JSON")
	}

	// Parse QoS
	qos, err := parseQoS(publishQoS)
	if err != nil {
		return err
	}

	// Parse target type
	targetType, err := parseTargetType(publishTargetType)
	if err != nil {
		return err
	}

	// Create envelope
	envelope := &umsv1.Envelope{
		Type:        publishType,
		Qos:         qos,
		Payload:     payload,
		OrgId:       orgID,
		RequiresAck: qos != umsv1.QoS_QOS_TELEMETRY,
	}

	// Create target
	target := &umsv1.Target{
		TargetType: targetType,
		TargetId:   publishTargetID,
		OrgId:      orgID,
	}

	// Create publisher
	publisher, err := sdk.NewPublisher(serverAddr)
	if err != nil {
		return fmt.Errorf("failed to create publisher: %w", err)
	}
	defer publisher.Close()

	// Publish with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if verbose {
		fmt.Printf("Publishing to %s: %s\n", serverAddr, publishType)
	}

	resp, err := publisher.Publish(ctx, envelope, []*umsv1.Target{target})
	if err != nil {
		return fmt.Errorf("failed to publish: %w", err)
	}

	// Print results
	fmt.Printf("Published envelope ID: %s\n", envelope.EnvelopeId)
	if len(resp.MailboxSeqs) > 0 {
		fmt.Printf("Mailbox sequences:\n")
		for mailboxID, seq := range resp.MailboxSeqs {
			fmt.Printf("  %s: %d\n", mailboxID, seq)
		}
	}

	return nil
}

func parseQoS(s string) (umsv1.QoS, error) {
	switch strings.ToLower(s) {
	case "command":
		return umsv1.QoS_QOS_COMMAND, nil
	case "control":
		return umsv1.QoS_QOS_CONTROL, nil
	case "telemetry":
		return umsv1.QoS_QOS_TELEMETRY, nil
	default:
		return umsv1.QoS_QOS_UNSPECIFIED, fmt.Errorf("invalid qos: %s (must be command, control, or telemetry)", s)
	}
}

func parseTargetType(s string) (umsv1.TargetType, error) {
	switch strings.ToLower(s) {
	case "server":
		return umsv1.TargetType_TARGET_TYPE_SERVER, nil
	case "cluster":
		return umsv1.TargetType_TARGET_TYPE_CLUSTER, nil
	case "org":
		return umsv1.TargetType_TARGET_TYPE_ORG, nil
	case "service":
		return umsv1.TargetType_TARGET_TYPE_SERVICE, nil
	case "broadcast":
		return umsv1.TargetType_TARGET_TYPE_BROADCAST, nil
	default:
		return umsv1.TargetType_TARGET_TYPE_UNSPECIFIED, fmt.Errorf("invalid target-type: %s", s)
	}
}
