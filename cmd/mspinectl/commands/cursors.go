package commands

import (
	"fmt"

	"github.com/ambientlabscomputing/adminclicore/flags"
	"github.com/ambientlabscomputing/adminclicore/ui"
	"github.com/ambientlabscomputing/mycelium_spine/proto/admin"
	"github.com/spf13/cobra"
)

// CursorsCmd returns the cursors command group.
func CursorsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "cursors",
		Short: "Manage mailbox cursors",
	}
	cmd.AddCommand(
		cursorsListCmd(),
		cursorsGetCmd(),
		cursorsResetCmd(),
		cursorsDeleteServerCmd(),
	)
	return cmd
}

func cursorsListCmd() *cobra.Command {
	var (
		serverID  string
		mailboxID string
		limit     int32
		offset    int32
	)
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List cursors",
		RunE: func(cmd *cobra.Command, args []string) error {
			d, err := getDeps(cmd)
			if err != nil {
				return err
			}
			resp, err := d.client.Cursors().List(d.ctx, &admin.ListCursorsRequest{
				ServerId:  serverID,
				MailboxId: mailboxID,
				Limit:     limit,
				Offset:    offset,
			})
			if err != nil {
				return fmt.Errorf("%s", ui.HandleGRPCError(err, ""))
			}
			switch d.printer.Format() {
			case ui.FormatJSON, ui.FormatYAML:
				return d.printer.PrintData(resp)
			default:
				for _, c := range resp.Cursors {
					fmt.Printf("  server_id=%-20s mailbox_id=%-36s last_acked_seq=%-6d updated_at=%s\n",
						c.ServerId, c.MailboxId, c.LastAckedSeq, c.UpdatedAt)
				}
				if resp.Pagination != nil {
					fmt.Printf("\nTotal: %d  Offset: %d  Limit: %d\n",
						resp.Pagination.Total, resp.Pagination.Offset, resp.Pagination.Limit)
				}
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&serverID, "server-id", "", "Filter by server ID")
	cmd.Flags().StringVar(&mailboxID, "mailbox-id", "", "Filter by mailbox ID")
	flags.RegisterPaginationFlags(cmd, &limit, &offset)
	return cmd
}

func cursorsGetCmd() *cobra.Command {
	var (
		serverID  string
		mailboxID string
	)
	cmd := &cobra.Command{
		Use:   "get",
		Short: "Get cursor detail",
		RunE: func(cmd *cobra.Command, args []string) error {
			d, err := getDeps(cmd)
			if err != nil {
				return err
			}
			resp, err := d.client.Cursors().Get(d.ctx, &admin.GetCursorRequest{
				ServerId:  serverID,
				MailboxId: mailboxID,
			})
			if err != nil {
				return fmt.Errorf("%s", ui.HandleGRPCError(err, ""))
			}
			switch d.printer.Format() {
			case ui.FormatJSON, ui.FormatYAML:
				return d.printer.PrintData(resp)
			default:
				d.printer.PrintKeyValue("Server ID", resp.ServerId)
				d.printer.PrintKeyValue("Mailbox ID", resp.MailboxId)
				d.printer.PrintKeyValue("Last Acked Seq", fmt.Sprintf("%d", resp.LastAckedSeq))
				d.printer.PrintKeyValue("Mailbox Next Seq", fmt.Sprintf("%d", resp.MailboxNextSeq))
				d.printer.PrintKeyValue("Inflight Count", fmt.Sprintf("%d", resp.InflightCount))
				d.printer.PrintKeyValue("Updated At", resp.UpdatedAt)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&serverID, "server-id", "", "Server ID")
	cmd.Flags().StringVar(&mailboxID, "mailbox-id", "", "Mailbox ID")
	_ = cmd.MarkFlagRequired("server-id")
	_ = cmd.MarkFlagRequired("mailbox-id")
	return cmd
}

func cursorsResetCmd() *cobra.Command {
	var (
		serverID  string
		mailboxID string
		seq       uint64
		yes       bool
	)
	cmd := &cobra.Command{
		Use:   "reset",
		Short: "Reset a cursor to a specific sequence number",
		RunE: func(cmd *cobra.Command, args []string) error {
			d, err := getDeps(cmd)
			if err != nil {
				return err
			}
			ok, err := flags.ConfirmOrSkip(d.printer, yes,
				fmt.Sprintf("Reset cursor server=%s mailbox=%s to seq=%d?", serverID, mailboxID, seq))
			if err != nil {
				return err
			}
			if !ok {
				d.printer.PrintWarning("Aborted.")
				return nil
			}
			if _, err := d.client.Cursors().Reset(d.ctx, &admin.ResetCursorRequest{
				ServerId:  serverID,
				MailboxId: mailboxID,
				Seq:       seq,
			}); err != nil {
				return fmt.Errorf("%s", ui.HandleGRPCError(err, ""))
			}
			d.printer.PrintSuccess(fmt.Sprintf("Cursor reset to seq %d.", seq))
			return nil
		},
	}
	cmd.Flags().StringVar(&serverID, "server-id", "", "Server ID")
	cmd.Flags().StringVar(&mailboxID, "mailbox-id", "", "Mailbox ID")
	cmd.Flags().Uint64Var(&seq, "seq", 0, "Sequence number to reset to")
	_ = cmd.MarkFlagRequired("server-id")
	_ = cmd.MarkFlagRequired("mailbox-id")
	_ = cmd.MarkFlagRequired("seq")
	flags.RegisterYesFlag(cmd, &yes)
	return cmd
}

func cursorsDeleteServerCmd() *cobra.Command {
	var (
		serverID string
		yes      bool
	)
	cmd := &cobra.Command{
		Use:   "delete-server",
		Short: "Delete all cursors for a server",
		RunE: func(cmd *cobra.Command, args []string) error {
			d, err := getDeps(cmd)
			if err != nil {
				return err
			}
			ok, err := flags.ConfirmOrSkip(d.printer, yes,
				fmt.Sprintf("Delete all cursors for server %s?", serverID))
			if err != nil {
				return err
			}
			if !ok {
				d.printer.PrintWarning("Aborted.")
				return nil
			}
			resp, err := d.client.Cursors().DeleteServerCursors(d.ctx, &admin.DeleteServerCursorsRequest{
				ServerId: serverID,
			})
			if err != nil {
				return fmt.Errorf("%s", ui.HandleGRPCError(err, ""))
			}
			d.printer.PrintSuccess(fmt.Sprintf("Deleted %d cursors for server %s.", resp.DeletedCount, serverID))
			return nil
		},
	}
	cmd.Flags().StringVar(&serverID, "server-id", "", "Server ID")
	_ = cmd.MarkFlagRequired("server-id")
	flags.RegisterYesFlag(cmd, &yes)
	return cmd
}
