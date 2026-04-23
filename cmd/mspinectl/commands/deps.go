// Package commands contains all mspinectl admin subcommand implementations.
package commands

import (
	"context"

	"github.com/ambientlabscomputing/adminclicore/skeleton"
	"github.com/ambientlabscomputing/adminclicore/ui"
	"github.com/ambientlabscomputing/mycelium_spine/cmd/mspinectl/client"
	"github.com/spf13/cobra"
)

// cmdDeps bundles per-command dependencies.
type cmdDeps struct {
	client  *client.AdminClient
	printer *ui.Printer
	ctx     context.Context
}

// getDeps extracts shared dependencies from the command context.
func getDeps(cmd *cobra.Command) (*cmdDeps, error) {
	d, err := skeleton.GetDeps(cmd)
	if err != nil {
		return nil, err
	}
	return &cmdDeps{
		client:  client.NewAdminClientFromConn(d.Conn),
		printer: d.Printer,
		ctx:     d.Ctx,
	}, nil
}
