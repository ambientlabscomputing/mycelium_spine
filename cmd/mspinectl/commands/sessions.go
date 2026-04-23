package commands

import (
	"fmt"

	"github.com/ambientlabscomputing/adminclicore/flags"
	"github.com/ambientlabscomputing/adminclicore/ui"
	"github.com/ambientlabscomputing/mycelium_spine/proto/admin"
	"github.com/spf13/cobra"
)

// SessionsCmd returns the sessions command group.
func SessionsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "sessions",
		Short: "Manage active sessions",
	}
	cmd.AddCommand(
		sessionsListCmd(),
		sessionsGetCmd(),
		sessionsDisconnectCmd(),
		sessionsEvictStaleCmd(),
	)
	return cmd
}

func sessionsListCmd() *cobra.Command {
	var (
		orgID  string
		limit  int32
		offset int32
	)
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List active sessions",
		RunE: func(cmd *cobra.Command, args []string) error {
			d, err := getDeps(cmd)
			if err != nil {
				return err
			}
			resp, err := d.client.Sessions().List(d.ctx, &admin.ListSessionsRequest{
				OrgId:  orgID,
				Limit:  limit,
				Offset: offset,
			})
			if err != nil {
				return fmt.Errorf("%s", ui.HandleGRPCError(err, ""))
			}
			switch d.printer.Format() {
			case ui.FormatJSON, ui.FormatYAML:
				return d.printer.PrintData(resp)
			default:
				for _, s := range resp.Sessions {
					fmt.Printf("  session_id=%-36s server_id=%-20s org_id=%s subs=%d connected_at=%s\n",
						s.SessionId, s.ServerId, s.OrgId, s.SubscriptionCount, s.ConnectedAt)
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
	flags.RegisterPaginationFlags(cmd, &limit, &offset)
	return cmd
}

func sessionsGetCmd() *cobra.Command {
	var (
		sessionID string
		serverID  string
	)
	cmd := &cobra.Command{
		Use:   "get",
		Short: "Get session detail",
		RunE: func(cmd *cobra.Command, args []string) error {
			d, err := getDeps(cmd)
			if err != nil {
				return err
			}
			resp, err := d.client.Sessions().Get(d.ctx, &admin.GetSessionRequest{
				SessionId: sessionID,
				ServerId:  serverID,
			})
			if err != nil {
				return fmt.Errorf("%s", ui.HandleGRPCError(err, ""))
			}
			switch d.printer.Format() {
			case ui.FormatJSON, ui.FormatYAML:
				return d.printer.PrintData(resp)
			default:
				d.printer.PrintKeyValue("Session ID", resp.SessionId)
				d.printer.PrintKeyValue("Server ID", resp.ServerId)
				d.printer.PrintKeyValue("Org ID", resp.OrgId)
				d.printer.PrintKeyValue("Epoch", fmt.Sprintf("%d", resp.SessionEpoch))
				d.printer.PrintKeyValue("Connected At", resp.ConnectedAt)
				d.printer.PrintKeyValue("Last Heartbeat", resp.LastHeartbeat)
				d.printer.PrintKeyValue("Device Fingerprint", resp.DeviceFingerprint)
				if len(resp.Subscriptions) > 0 {
					fmt.Println("\nSubscriptions:")
					for _, s := range resp.Subscriptions {
						fmt.Printf("  %s\n", s)
					}
				}
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&sessionID, "session-id", "", "Session ID")
	cmd.Flags().StringVar(&serverID, "server-id", "", "Server ID")
	return cmd
}

func sessionsDisconnectCmd() *cobra.Command {
	var (
		sessionID string
		serverID  string
		yes       bool
	)
	cmd := &cobra.Command{
		Use:   "disconnect",
		Short: "Disconnect a session",
		RunE: func(cmd *cobra.Command, args []string) error {
			d, err := getDeps(cmd)
			if err != nil {
				return err
			}
			ok, err := flags.ConfirmOrSkip(d.printer, yes, fmt.Sprintf("Disconnect session %s?", sessionID))
			if err != nil {
				return err
			}
			if !ok {
				d.printer.PrintWarning("Aborted.")
				return nil
			}
			if _, err := d.client.Sessions().Disconnect(d.ctx, &admin.DisconnectSessionRequest{
				SessionId: sessionID,
				ServerId:  serverID,
			}); err != nil {
				return fmt.Errorf("%s", ui.HandleGRPCError(err, ""))
			}
			d.printer.PrintSuccess(fmt.Sprintf("Session %s disconnected.", sessionID))
			return nil
		},
	}
	cmd.Flags().StringVar(&sessionID, "session-id", "", "Session ID to disconnect")
	cmd.Flags().StringVar(&serverID, "server-id", "", "Server ID")
	_ = cmd.MarkFlagRequired("session-id")
	flags.RegisterYesFlag(cmd, &yes)
	return cmd
}

func sessionsEvictStaleCmd() *cobra.Command {
	var yes bool
	cmd := &cobra.Command{
		Use:   "evict-stale",
		Short: "Evict stale sessions that have missed heartbeats",
		RunE: func(cmd *cobra.Command, args []string) error {
			d, err := getDeps(cmd)
			if err != nil {
				return err
			}
			ok, err := flags.ConfirmOrSkip(d.printer, yes, "Evict all stale sessions?")
			if err != nil {
				return err
			}
			if !ok {
				d.printer.PrintWarning("Aborted.")
				return nil
			}
			resp, err := d.client.Sessions().EvictStale(d.ctx, &admin.Empty{})
			if err != nil {
				return fmt.Errorf("%s", ui.HandleGRPCError(err, ""))
			}
			d.printer.PrintSuccess(fmt.Sprintf("Evicted %d stale sessions.", resp.EvictedCount))
			return nil
		},
	}
	flags.RegisterYesFlag(cmd, &yes)
	return cmd
}
