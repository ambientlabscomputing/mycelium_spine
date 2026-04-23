// Package cli contains the root Cobra command and Execute entry point for mspinectl.
package cli

import (
	"context"

	"github.com/ambientlabscomputing/adminclicore/skeleton"
	"github.com/ambientlabscomputing/mycelium_spine/cmd/mspinectl/cmd"
	"github.com/ambientlabscomputing/mycelium_spine/cmd/mspinectl/commands"
)

// Execute builds the root command and runs it.
func Execute(ctx context.Context, version string) error {
	root := skeleton.NewRootCommand(skeleton.AppConfig{
		Name:          "mspinectl",
		Short:         "mspinectl -- admin CLI for Mycelium Spine",
		Version:       version,
		DefaultSocket: "/tmp/spine_admin.sock",
	})

	root.AddCommand(
		commands.HealthCmd(),
		commands.SessionsCmd(),
		commands.MailboxesCmd(),
		commands.CursorsCmd(),
		commands.WorkersCmd(),
		cmd.ClientCmds(),
		cmd.SubscribeCmd(),
		cmd.PublishCmd(),
		cmd.AckCmd(),
	)

	return root.ExecuteContext(ctx)
}
