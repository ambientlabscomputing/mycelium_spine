package commands

import (
	"fmt"

	"github.com/ambientlabscomputing/adminclicore/flags"
	"github.com/ambientlabscomputing/adminclicore/ui"
	"github.com/ambientlabscomputing/mycelium_spine/proto/admin"
	"github.com/spf13/cobra"
)

// MailboxesCmd returns the mailboxes command group.
func MailboxesCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "mailboxes",
		Short: "Manage mailboxes",
	}
	cmd.AddCommand(
		mailboxesListCmd(),
		mailboxesGetCmd(),
		mailboxesListEnvelopesCmd(),
		mailboxesPurgeExpiredCmd(),
		mailboxesClearOutboxCmd(),
	)
	return cmd
}

func mailboxesListCmd() *cobra.Command {
	var (
		orgID      string
		targetType string
		limit      int32
		offset     int32
	)
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List mailboxes",
		RunE: func(cmd *cobra.Command, args []string) error {
			d, err := getDeps(cmd)
			if err != nil {
				return err
			}
			resp, err := d.client.Mailboxes().List(d.ctx, &admin.ListMailboxesRequest{
				OrgId:      orgID,
				TargetType: targetType,
				Limit:      limit,
				Offset:     offset,
			})
			if err != nil {
				return fmt.Errorf("%s", ui.HandleGRPCError(err, ""))
			}
			switch d.printer.Format() {
			case ui.FormatJSON, ui.FormatYAML:
				return d.printer.PrintData(resp)
			default:
				for _, m := range resp.Mailboxes {
					fmt.Printf("  mailbox_id=%-36s type=%-10s target_id=%-20s org_id=%s next_seq=%d\n",
						m.MailboxId, m.TargetType, m.TargetId, m.OrgId, m.NextSeq)
				}
				if resp.Pagination != nil {
					fmt.Printf("\nTotal: %d  Offset: %d  Limit: %d\n",
						resp.Pagination.Total, resp.Pagination.Offset, resp.Pagination.Limit)
				}
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&orgID, "org-id", "", "Filter by org ID")
	cmd.Flags().StringVar(&targetType, "target-type", "", "Filter by target type")
	flags.RegisterPaginationFlags(cmd, &limit, &offset)
	return cmd
}

func mailboxesGetCmd() *cobra.Command {
	var mailboxID string
	cmd := &cobra.Command{
		Use:   "get",
		Short: "Get mailbox detail",
		RunE: func(cmd *cobra.Command, args []string) error {
			d, err := getDeps(cmd)
			if err != nil {
				return err
			}
			resp, err := d.client.Mailboxes().Get(d.ctx, &admin.GetMailboxRequest{
				MailboxId: mailboxID,
			})
			if err != nil {
				return fmt.Errorf("%s", ui.HandleGRPCError(err, ""))
			}
			switch d.printer.Format() {
			case ui.FormatJSON, ui.FormatYAML:
				return d.printer.PrintData(resp)
			default:
				d.printer.PrintKeyValue("Mailbox ID", resp.MailboxId)
				d.printer.PrintKeyValue("Target Type", resp.TargetType)
				d.printer.PrintKeyValue("Target ID", resp.TargetId)
				d.printer.PrintKeyValue("Org ID", resp.OrgId)
				d.printer.PrintKeyValue("Next Seq", fmt.Sprintf("%d", resp.NextSeq))
				d.printer.PrintKeyValue("Retention (s)", fmt.Sprintf("%d", resp.RetentionSeconds))
				d.printer.PrintKeyValue("Max Envelopes", fmt.Sprintf("%d", resp.MaxEnvelopes))
				d.printer.PrintKeyValue("Created At", resp.CreatedAt)
				d.printer.PrintKeyValue("Updated At", resp.UpdatedAt)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&mailboxID, "mailbox-id", "", "Mailbox ID")
	_ = cmd.MarkFlagRequired("mailbox-id")
	return cmd
}

func mailboxesListEnvelopesCmd() *cobra.Command {
	var (
		mailboxID string
		fromSeq   uint64
		limit     int32
	)
	cmd := &cobra.Command{
		Use:   "list-envelopes",
		Short: "List envelopes in a mailbox",
		RunE: func(cmd *cobra.Command, args []string) error {
			d, err := getDeps(cmd)
			if err != nil {
				return err
			}
			resp, err := d.client.Mailboxes().ListEnvelopes(d.ctx, &admin.ListEnvelopesRequest{
				MailboxId: mailboxID,
				FromSeq:   fromSeq,
				Limit:     limit,
			})
			if err != nil {
				return fmt.Errorf("%s", ui.HandleGRPCError(err, ""))
			}
			switch d.printer.Format() {
			case ui.FormatJSON, ui.FormatYAML:
				return d.printer.PrintData(resp)
			default:
				for _, e := range resp.Envelopes {
					fmt.Printf("  seq=%-6d type=%-30s qos=%-10s requires_ack=%v expires_at_ms=%d\n",
						e.Seq, e.Type, e.Qos, e.RequiresAck, e.ExpiresAtMs)
				}
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&mailboxID, "mailbox-id", "", "Mailbox ID")
	cmd.Flags().Uint64Var(&fromSeq, "from-seq", 0, "Start listing from this sequence number")
	cmd.Flags().Int32Var(&limit, "limit", 20, "Max envelopes to return")
	_ = cmd.MarkFlagRequired("mailbox-id")
	return cmd
}

func mailboxesPurgeExpiredCmd() *cobra.Command {
	var (
		mailboxID string
		yes       bool
	)
	cmd := &cobra.Command{
		Use:   "purge-expired",
		Short: "Purge expired envelopes from a mailbox",
		RunE: func(cmd *cobra.Command, args []string) error {
			d, err := getDeps(cmd)
			if err != nil {
				return err
			}
			ok, err := flags.ConfirmOrSkip(d.printer, yes, fmt.Sprintf("Purge expired envelopes from mailbox %s?", mailboxID))
			if err != nil {
				return err
			}
			if !ok {
				d.printer.PrintWarning("Aborted.")
				return nil
			}
			resp, err := d.client.Mailboxes().PurgeExpired(d.ctx, &admin.PurgeExpiredRequest{
				MailboxId: mailboxID,
			})
			if err != nil {
				return fmt.Errorf("%s", ui.HandleGRPCError(err, ""))
			}
			d.printer.PrintSuccess(fmt.Sprintf("Purged %d expired envelopes.", resp.DeletedCount))
			return nil
		},
	}
	cmd.Flags().StringVar(&mailboxID, "mailbox-id", "", "Mailbox ID")
	_ = cmd.MarkFlagRequired("mailbox-id")
	flags.RegisterYesFlag(cmd, &yes)
	return cmd
}

func mailboxesClearOutboxCmd() *cobra.Command {
	var (
		mailboxID string
		yes       bool
	)
	cmd := &cobra.Command{
		Use:   "clear-outbox",
		Short: "Clear all envelopes from a mailbox outbox",
		RunE: func(cmd *cobra.Command, args []string) error {
			d, err := getDeps(cmd)
			if err != nil {
				return err
			}
			ok, err := flags.ConfirmOrSkip(d.printer, yes, fmt.Sprintf("Clear all envelopes from mailbox %s?", mailboxID))
			if err != nil {
				return err
			}
			if !ok {
				d.printer.PrintWarning("Aborted.")
				return nil
			}
			resp, err := d.client.Mailboxes().ClearOutbox(d.ctx, &admin.ClearOutboxRequest{
				MailboxId: mailboxID,
			})
			if err != nil {
				return fmt.Errorf("%s", ui.HandleGRPCError(err, ""))
			}
			d.printer.PrintSuccess(fmt.Sprintf("Cleared %d envelopes from mailbox %s.", resp.DeletedCount, mailboxID))
			return nil
		},
	}
	cmd.Flags().StringVar(&mailboxID, "mailbox-id", "", "Mailbox ID")
	_ = cmd.MarkFlagRequired("mailbox-id")
	flags.RegisterYesFlag(cmd, &yes)
	return cmd
}
