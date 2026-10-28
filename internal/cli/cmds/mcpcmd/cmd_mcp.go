package mcpcmd

import (
	"fmt"

	"github.com/opengfx1030/bernie/internal/cli"
	"github.com/opengfx1030/bernie/internal/mcp"
)

func init() {
	cmd := &cli.Command{
		Name:    "mcp",
		Summary: "Serve MCP (Model Context Protocol) to an agent on stdin and stdout",
		Usage:   "",
	}
	cmd.Run = func(c *cli.Context, args []string) error {
		pos, err := cli.Parse(c.Flags(cmd), args)
		if err != nil {
			return err
		}
		if len(pos) > 0 {
			return cli.Usagef("unexpected argument %q", pos[0])
		}
		// stdout carries only protocol messages; diagnostics go to stderr.
		srv := mcp.New(mcp.Options{Transport: "stdio", Version: cli.Version, Log: c.Err})
		fmt.Fprintf(c.Err, "bernie mcp: serving on stdio (read-only)\n")
		if err := srv.Run(c.Ctx); err != nil && c.Ctx.Err() == nil {
			return err
		}
		return nil
	}
	cli.Register(cmd)
}
