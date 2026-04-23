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

func newPublishCmd(f *clientFlags) *cobra.Command {
	var (
		msgType    string
		qosStr     string
		data       string
		file       string
		targetType string
		targetID   string
	)

	cmd := &cobra.Command{
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
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := checkRequired("server-id", f.serverID); err != nil {
				return err
			}
			if err := checkRequired("org-id", f.orgID); err != nil {
				return err
			}

			var payload []byte
			if data != "" {
				payload = []byte(data)
			} else if file != "" {
				b, err := os.ReadFile(file)
				if err != nil {
					return fmt.Errorf("failed to read file: %w", err)
				}
				payload = b
			} else {
				return fmt.Errorf("either --data or --file must be provided")
			}

			if !json.Valid(payload) {
				return fmt.Errorf("payload is not valid JSON")
			}

			qos, err := parseQoS(qosStr)
			if err != nil {
				return err
			}

			tt, err := parseUMSTargetType(targetType)
			if err != nil {
				return err
			}

			envelope := &umsv1.Envelope{
				Type:        msgType,
				Qos:         qos,
				Payload:     payload,
				OrgId:       f.orgID,
				RequiresAck: qos != umsv1.QoS_QOS_TELEMETRY,
			}

			target := &umsv1.Target{
				TargetType: tt,
				TargetId:   targetID,
				OrgId:      f.orgID,
			}

			publisher, err := sdk.NewPublisher(f.serverAddr)
			if err != nil {
				return fmt.Errorf("failed to create publisher: %w", err)
			}
			defer publisher.Close()

			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()

			if f.verbose {
				fmt.Printf("Publishing to %s: %s\n", f.serverAddr, msgType)
			}

			resp, err := publisher.Publish(ctx, envelope, []*umsv1.Target{target})
			if err != nil {
				return fmt.Errorf("failed to publish: %w", err)
			}

			fmt.Printf("Published envelope ID: %s\n", envelope.EnvelopeId)
			if len(resp.MailboxSeqs) > 0 {
				fmt.Printf("Mailbox sequences:\n")
				for mailboxID, seq := range resp.MailboxSeqs {
					fmt.Printf("  %s: %d\n", mailboxID, seq)
				}
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&msgType, "type", "", "Message type (e.g., command.deploy)")
	cmd.Flags().StringVar(&qosStr, "qos", "control", "QoS level: command, control, or telemetry")
	cmd.Flags().StringVar(&data, "data", "", "JSON payload data")
	cmd.Flags().StringVar(&file, "file", "", "File containing JSON payload")
	cmd.Flags().StringVar(&targetType, "target-type", "", "Target type: server, cluster, org, service, or broadcast")
	cmd.Flags().StringVar(&targetID, "target-id", "", "Target ID")
	_ = cmd.MarkFlagRequired("type")
	_ = cmd.MarkFlagRequired("target-type")

	return cmd
}

func parseQoS(s string) (umsv1.QoS, error) {
	switch strings.ToUpper(s) {
	case "COMMAND":
		return umsv1.QoS_QOS_COMMAND, nil
	case "CONTROL":
		return umsv1.QoS_QOS_CONTROL, nil
	case "TELEMETRY":
		return umsv1.QoS_QOS_TELEMETRY, nil
	default:
		return umsv1.QoS_QOS_UNSPECIFIED, fmt.Errorf("unknown QoS %q", s)
	}
}
