// Command rdnactl sets up, checks, tunes, serves and benchmarks vLLM on
// AMD ROCm/RDNA machines. Every feature is a subcommand; the dashboard
// (`rdnactl dash`) is a view over the same packages.
package main

import (
	"os"

	"github.com/opengfx1030/rdnactl/internal/cli"
)

func main() {
	os.Exit(cli.Main(os.Args[1:]))
}
