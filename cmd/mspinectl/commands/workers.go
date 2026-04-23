package commands

import (
	"fmt"

	"github.com/ambientlabscomputing/adminclicore/ui"
	"github.com/ambientlabscomputing/mycelium_spine/proto/admin"
	"github.com/spf13/cobra"
)

// WorkersCmd returns the workers command group.
func WorkersCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "workers",
		Short: "Worker pool diagnostics",
	}
	cmd.AddCommand(workersStatsCmd())
	return cmd
}

func workersStatsCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "stats",
		Short: "Show worker pool statistics",
		RunE: func(cmd *cobra.Command, args []string) error {
			d, err := getDeps(cmd)
			if err != nil {
				return err
			}
			resp, err := d.client.Workers().GetStats(d.ctx, &admin.Empty{})
			if err != nil {
				return fmt.Errorf("%s", ui.HandleGRPCError(err, "is mycelium_spine running with admin socket enabled?"))
			}
			switch d.printer.Format() {
			case ui.FormatJSON, ui.FormatYAML:
				return d.printer.PrintData(resp)
			default:
				if len(resp.Pools) == 0 {
					d.printer.PrintInfo("No worker pools found.")
					return nil
				}
				for poolName, p := range resp.Pools {
					fmt.Printf("[%s]\n", poolName)
					d.printer.PrintKeyValue("  Active Workers", fmt.Sprintf("%d", p.ActiveWorkers))
					d.printer.PrintKeyValue("  Queue Depth", fmt.Sprintf("%d / %d", p.QueueDepth, p.QueueCapacity))
					d.printer.PrintKeyValue("  Tasks Processed", fmt.Sprintf("%d", p.TasksProcessed))
					d.printer.PrintKeyValue("  Task Errors", fmt.Sprintf("%d", p.TaskErrors))
					fmt.Println()
				}
			}
			return nil
		},
	}
}
