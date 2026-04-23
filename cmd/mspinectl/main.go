package main

import (
	"context"
	"fmt"
	"os"

	"github.com/ambientlabscomputing/mycelium_spine/cmd/mspinectl/cli"
)

// version is overridden at link time via -X main.version=<tag>.
var version = "dev"

func main() {
	ctx := context.Background()
	if err := cli.Execute(ctx, version); err != nil {
		fmt.Fprintln(os.Stderr, "Error:", err)
		os.Exit(1)
	}
}
