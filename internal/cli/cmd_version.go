package cli

import (
	"fmt"
	"io"
	"runtime"
	"runtime/debug"
)

// Version is set at build time: -ldflags "-X github.com/opengfx1030/bernie/internal/cli.Version=v0.1.0".
var Version = "dev"

func init() {
	cmd := &Command{Name: "version", Summary: "Print the bernie version"}
	cmd.Run = func(c *Context, args []string) error {
		commit := ""
		if bi, ok := debug.ReadBuildInfo(); ok {
			for _, s := range bi.Settings {
				if s.Key == "vcs.revision" {
					commit = s.Value
				}
			}
		}
		v := map[string]string{"version": Version, "commit": commit, "go": runtime.Version(), "platform": runtime.GOOS + "/" + runtime.GOARCH}
		return c.Emit(v, func(w io.Writer) {
			fmt.Fprintf(w, "bernie %s (%s, %s)\n", Version, v["platform"], runtime.Version())
		})
	}
	Register(cmd)
}
