package mcpcmd

import (
	"fmt"
	"os"

	"github.com/opengfx1030/bernie/internal/cli"
	"github.com/opengfx1030/bernie/internal/mcp"
)

func init() {
	cmd := &cli.Command{
		Name:    "mcp",
		Summary: "Serve MCP (Model Context Protocol) to an agent on stdin and stdout",
		Usage:   "[--allow-write]",
	}
	cmd.Run = func(c *cli.Context, args []string) error {
		fs := c.Flags(cmd)
		write := fs.Bool("allow-write", os.Getenv("BERNIE_MCP_WRITE") == "1", "also expose the tools that change the host (serve, stop, bench_run, bench_ab, setup_apply, setup_verify, tune_power_set); default: BERNIE_MCP_WRITE=1")
		pos, err := cli.Parse(fs, args)
		if err != nil {
			return err
		}
		if len(pos) > 0 {
			return cli.Usagef("unexpected argument %q", pos[0])
		}
		// stdout carries only protocol messages; diagnostics go to stderr.
		srv := mcp.New(mcp.Options{Write: *write, Transport: "stdio", Version: cli.Version, Log: c.Err})
		mode := "read-only"
		if *write {
			mode = "write enabled"
		}
		fmt.Fprintf(c.Err, "bernie mcp: serving on stdio (%s)\n", mode)
		if err := srv.Run(c.Ctx); err != nil && c.Ctx.Err() == nil {
			return err
		}
		return nil
	}
	cli.Register(cmd)
}
