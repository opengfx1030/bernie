// Command bernie sets up, checks, tunes, serves and benchmarks vLLM on
// AMD ROCm/RDNA machines. Every feature is a subcommand; the dashboard
// (`bernie dash`) is a view over the same packages.
package main

import (
	"os"

	"github.com/opengfx1030/bernie/internal/cli"
	_ "github.com/opengfx1030/bernie/internal/cli/cmds/benchcmd"
	_ "github.com/opengfx1030/bernie/internal/cli/cmds/dashcmd"
	_ "github.com/opengfx1030/bernie/internal/cli/cmds/health"
	_ "github.com/opengfx1030/bernie/internal/cli/cmds/inventory"
	_ "github.com/opengfx1030/bernie/internal/cli/cmds/serving"
	_ "github.com/opengfx1030/bernie/internal/cli/cmds/tunecmd"
)

func main() {
	os.Exit(cli.Main(os.Args[1:]))
}
