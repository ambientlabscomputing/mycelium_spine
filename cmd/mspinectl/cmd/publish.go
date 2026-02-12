package cmd
package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/ambientlabscomputing/mycelium_spine/sdk"
	umsv1 "github.com/ambientlabscomputing/mycelium_spine/proto/ums/v1"
	"github.com/spf13/cobra"
)

var (
	publishType      string
	publishQoS       string
	publishData      string
	publishFile      string




























































































































































}	}		return umsv1.TargetType_TARGET_TYPE_UNSPECIFIED, fmt.Errorf("invalid target-type: %s (must be: server, cluster, org, service, broadcast)", targetType)	default:		return umsv1.TargetType_TARGET_TYPE_BROADCAST, nil	case "broadcast":		return umsv1.TargetType_TARGET_TYPE_SERVICE, nil	case "service":		return umsv1.TargetType_TARGET_TYPE_ORG, nil	case "org":		return umsv1.TargetType_TARGET_TYPE_CLUSTER, nil	case "cluster":		return umsv1.TargetType_TARGET_TYPE_SERVER, nil	case "server":	switch targetType {func parseTargetType(targetType string) (umsv1.TargetType, error) {}	}		return umsv1.QoS_QOS_UNSPECIFIED, fmt.Errorf("invalid qos: %s (must be: command, control, telemetry)", qos)	default:		return umsv1.QoS_QOS_TELEMETRY, nil	case "telemetry":		return umsv1.QoS_QOS_CONTROL, nil	case "control":		return umsv1.QoS_QOS_COMMAND, nil	case "command":	switch qos {func parseQoS(qos string) (umsv1.QoS, error) {}	return nil	}		fmt.Printf("    %s: %d\n", mailboxID, seq)	for mailboxID, seq := range resp.MailboxSeqs {	fmt.Printf("  Mailbox sequences:\n")	fmt.Printf("  Envelope ID: %s\n", envelope.EnvelopeId)	fmt.Printf("✓ Published successfully\n")	// Print result	}		return fmt.Errorf("publish failed: %w", err)	if err != nil {	resp, err := publisher.Publish(ctx, envelope, targets)	}		fmt.Printf("Publishing to %s:%s...\n", publishTargetType, publishTargetID)	if verbose {	defer cancel()	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(publishTimeout)*time.Second)	// Publish with timeout	}		RequiresAck: qos != umsv1.QoS_QOS_TELEMETRY,		OrgId:       orgID,		Payload:     payload,		Qos:         qos,		Type:        publishType,	envelope := &umsv1.Envelope{	// Create envelope	}		},			OrgId:      orgID,			TargetId:   publishTargetID,			TargetType: targetType,		{	targets := []*umsv1.Target{	// Create target	defer publisher.Close()	}		return fmt.Errorf("failed to create publisher: %w", err)	if err != nil {	publisher, err := sdk.NewPublisher(serverAddr)	// Create publisher	}		return err	if err != nil {	targetType, err := parseTargetType(publishTargetType)	// Parse target type	}		return err	if err != nil {	qos, err := parseQoS(publishQoS)	// Parse QoS	}		return fmt.Errorf("payload is not valid JSON")	if !json.Valid(payload) {	// Validate JSON	}		return fmt.Errorf("either --data or --file must be specified")	} else {		payload = []byte(publishData)	} else if publishData != "" {		}			return fmt.Errorf("failed to read file: %w", err)		if err != nil {		payload, err = os.ReadFile(publishFile)	if publishFile != "" {	var err error	var payload []byte	// Get payload	}		return err	if err := checkRequired("org-id", orgID); err != nil {	}		return err	if err := checkRequired("server-id", serverID); err != nil {	// Validate required flagsfunc runPublish(cmd *cobra.Command, args []string) error {}	publishCmd.MarkFlagRequired("target-id")	publishCmd.MarkFlagRequired("target-type")	publishCmd.MarkFlagRequired("type")	publishCmd.Flags().IntVar(&publishTimeout, "timeout", 10, "Publish timeout in seconds")	publishCmd.Flags().StringVar(&publishTargetID, "target-id", "", "Target ID")	publishCmd.Flags().StringVar(&publishTargetType, "target-type", "", "Target type: server, cluster, org, service, broadcast")	publishCmd.Flags().StringVar(&publishFile, "file", "", "Read payload from file")	publishCmd.Flags().StringVar(&publishData, "data", "", "Message payload (JSON string)")	publishCmd.Flags().StringVar(&publishQoS, "qos", "command", "QoS level: command, control, telemetry")	publishCmd.Flags().StringVar(&publishType, "type", "", "Envelope type (e.g., command.deploy, config.update)")	rootCmd.AddCommand(publishCmd)func init() {}	RunE: runPublish,  mspinectl publish --type config.update --qos control --target-type cluster --target-id cluster-west --data '{"key":"value"}'`,  # Publish to cluster  mspinectl publish --type command.deploy --qos command --target-type server --target-id prod-01 --file ./deploy.json  # Publish from file  mspinectl publish --type command.deploy --qos command --target-type server --target-id prod-01 --data '{"version":"v2.0"}'	Example: `  # Publish a command to a serverThe message will be delivered to all subscribers of the target mailboxes.`,	Long: `Publish an envelope to one or more targets in Mycelium Spine.	Short: "Publish a message to a target",	Use:   "publish",var publishCmd = &cobra.Command{)	publishTimeout   int	publishTargetID  string	publishTargetType string