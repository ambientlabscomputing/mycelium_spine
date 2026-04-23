package commands

import (
	"fmt"

	"github.com/ambientlabscomputing/adminclicore/ui"
	"github.com/ambientlabscomputing/mycelium_spine/proto/admin"
	"github.com/spf13/cobra"
)

// HealthCmd returns the health command group.
func HealthCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "health",
		Short: "Health checks and diagnostics",
	}
	cmd.AddCommand(healthCheckCmd(), healthDetailCmd())
	return cmd
}

func healthCheckCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "check",
		Short: "Quick status check",
		RunE: func(cmd *cobra.Command, args []string) error {
			d, err := getDeps(cmd)
			if err != nil {
				return err
			}
			resp, err := d.client.Health().Check(d.ctx, &admin.Empty{})
			if err != nil {
				return fmt.Errorf("%s", ui.HandleGRPCError(err, "is mycelium_spine running with admin socket enabled?"))
			}
			switch d.printer.Format() {
			case ui.FormatJSON, ui.FormatYAML:
				return d.printer.PrintData(resp)
			default:
				d.printer.PrintKeyValue("Status", ui.ColorStatus(resp.Status))
				d.printer.PrintKeyValue("Sessions", fmt.Sprintf("%d", resp.ActiveSessions))
				d.printer.PrintKeyValue("Mailboxes", fmt.Sprintf("%d", resp.TotalMailboxes))
				d.printer.PrintKeyValue("Uptime", resp.Uptime)
				d.printer.PrintKeyValue("Version", resp.Version)
			}
			return nil
		},
	}
}

func healthDetailCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "detail",
		Short: "Detailed diagnostics including worker pool stats",
		RunE: func(cmd *cobra.Command, args []string) error {
			d, err := getDeps(cmd)
			if err != nil {
				return err
			}
			resp, err := d.client.Health().Detail(d.ctx, &admin.Empty{})
			if err != nil {
				return fmt.Errorf("%s", ui.HandleGRPCError(err, "is mycelium_spine running with admin socket enabled?"))
			}
			switch d.printer.Format() {
			case ui.FormatJSON, ui.FormatYAML:
				return d.printer.PrintData(resp)
			default:
				d.printer.PrintKeyValue("Status", ui.ColorStatus(resp.Status))
				d.printer.PrintKeyValue("Sessions", fmt.Sprintf("%d", resp.ActiveSessions))
				d.printer.PrintKeyValue("Mailboxes", fmt.Sprintf("%d", resp.TotalMailboxes))
				d.printer.PrintKeyValue("Uptime", resp.Uptime)
				d.printer.PrintKeyValue("Version", resp.Version)
				if len(resp.WorkerPools) > 0 {
					fmt.Println()
					fmt.Println("Worker Pools:")
					for poolName, p := range resp.WorkerPools {
						fmt.Printf("  [%s] active=%d queue=%d/%d processed=%d errors=%d\n",
							poolName, p.ActiveWorkers, p.QueueDepth, p.QueueCapacity,
							p.TasksProcessed, p.TaskErrors)
					}
				}
			}
			return nil
		},
	}
}
