package tunecmd

import (
	"fmt"
	"github.com/opengfx1030/rdnactl/internal/cli"
	"io"
	"os"
	"sort"
	"strconv"
	"strings"
	"text/tabwriter"

	"github.com/opengfx1030/rdnactl/internal/config"
	"github.com/opengfx1030/rdnactl/internal/gpu"
	"github.com/opengfx1030/rdnactl/internal/rocm"
	"github.com/opengfx1030/rdnactl/internal/tuning"
)

func init() {
	tune := &cli.Command{Name: "tune", Summary: "TunableOp rows and GPU power cap"}
	top := &cli.Command{Name: "tunableop", Summary: "TunableOp profiles (rows per rocBLAS build)"}

	list := &cli.Command{Name: "list", Summary: "Registered TunableOp profiles"}
	list.Run = func(c *cli.Context, args []string) error {
		if _, err := cli.Parse(c.Flags(list), args); err != nil {
			return err
		}
		ps, err := tuning.Profiles()
		if err != nil {
			return err
		}
		return c.Emit(ps, func(w io.Writer) {
			tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
			fmt.Fprintln(tw, "NAME\tARCH\tROCBLAS\tROCM\tTORCH\tLIB SHA256\tROWS/RANK\tRANKS\tSTATUS")
			for _, p := range ps {
				fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\t%d\t%d\t%s\n", p.Name, strings.Join(p.Arch, ","), p.ROCBLAS, p.ROCm, p.Torch,
					p.LibSHA256[:tuning.SHAPrefix], p.RowsPerRank, len(p.Ranks), firstWords(p.Status, 6))
			}
			tw.Flush()
		})
	}

	status := &cli.Command{Name: "status", Summary: "Loaded rocBLAS build and its matching profile", Usage: "[--venv DIR]"}
	status.Run = func(c *cli.Context, args []string) error {
		fs := c.Flags(status)
		venvFlag := fs.String("venv", "", "venv (default: $RDNACTL_VENV, config venv, $VIRTUAL_ENV)")
		if _, err := cli.Parse(fs, args); err != nil {
			return err
		}
		st, err := tunableOpStatus(*venvFlag)
		if err != nil {
			return err
		}
		return c.Emit(st, func(w io.Writer) {
			fmt.Fprintf(w, "venv:     %s\n", orNone(st.Venv))
			fmt.Fprintf(w, "rocblas:  %s\n", orNone(st.ROCBLAS))
			if st.SHA256 != "" {
				fmt.Fprintf(w, "sha256:   %s\n", st.SHA256)
			}
			fmt.Fprintf(w, "gpus:     %s\n", orNone(strings.Join(st.Archs, ", ")))
			if st.Profile != "" {
				fmt.Fprintf(w, "profile:  %s (rows: %s)\n", st.Profile, tuning.RowsDir(st.Profile))
			} else {
				fmt.Fprintf(w, "profile:  none; per-user rows %s\n", tuning.PerUserFile())
			}
		})
	}

	env := &cli.Command{Name: "env", Summary: "export lines for TunableOp (materializes the rows)", Usage: "[--venv DIR] [--profile P] [--tuning]"}
	env.Run = func(c *cli.Context, args []string) error {
		fs := c.Flags(env)
		venvFlag := fs.String("venv", "", "venv (default: $RDNACTL_VENV, config venv, $VIRTUAL_ENV)")
		profile := fs.String("profile", "", "explicit profile (validated against the loaded rocBLAS)")
		tune := fs.Bool("tuning", false, "online tuning into the per-user rows file")
		if _, err := cli.Parse(fs, args); err != nil {
			return err
		}
		st, err := tunableOpStatus(*venvFlag)
		if err != nil {
			return err
		}
		if st.ROCBLAS == "" && *profile == "" {
			return fmt.Errorf("librocblas.so.5 not found (venv wheel or system); give --profile or --venv")
		}
		p, err := tuning.Select(st.SHA256, *profile, st.Archs)
		if err != nil {
			return err
		}
		dir := ""
		if p != nil {
			if dir, err = tuning.Materialize(p); err != nil {
				return err
			}
		} else {
			c.Logf("no profile matches librocblas.so.5 %s on %s; per-user rows", short12(st.SHA256), orNone(strings.Join(st.Archs, ", ")))
		}
		e := tuning.Env(p, dir, *tune)
		return c.Emit(e, func(w io.Writer) {
			keys := make([]string, 0, len(e))
			for k := range e {
				keys = append(keys, k)
			}
			sort.Strings(keys)
			for _, k := range keys {
				fmt.Fprintf(w, "export %s=%s\n", k, e[k])
			}
		})
	}
	top.Subs = []*cli.Command{list, status, env}

	power := &cli.Command{Name: "power", Summary: "GPU power cap"}
	pstatus := &cli.Command{Name: "status", Summary: "Power cap per GPU"}
	pstatus.Run = func(c *cli.Context, args []string) error {
		if _, err := cli.Parse(c.Flags(pstatus), args); err != nil {
			return err
		}
		gpus, err := gpu.List("/")
		if err != nil {
			return err
		}
		ps := tuning.PowerStatus(gpus)
		return c.Emit(ps, func(w io.Writer) {
			tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
			fmt.Fprintln(tw, "GPU\tNAME\tCAP W\tMIN W\tMAX W")
			for _, p := range ps {
				fmt.Fprintf(tw, "%d\t%s\t%.0f\t%.0f\t%.0f\n", p.HIPIndex, p.Name, p.CapW, p.MinW, p.MaxW)
			}
			tw.Flush()
		})
	}
	pset := &cli.Command{Name: "set", Summary: "Set the power cap (prints the command without root)", Usage: "<WATTS> [--gpu IDX]..."}
	pset.Run = func(c *cli.Context, args []string) error {
		fs := c.Flags(pset)
		var idxFlags cli.MultiFlag
		fs.Var(&idxFlags, "gpu", "HIP index, repeatable (default: all GPUs)")
		pos, err := cli.Parse(fs, args)
		if err != nil {
			return err
		}
		if len(pos) != 1 {
			return cli.Usagef("tune power set needs WATTS")
		}
		watts, err := strconv.ParseFloat(pos[0], 64)
		if err != nil || watts <= 0 {
			return cli.Usagef("WATTS: %q is not a positive number", pos[0])
		}
		var idx []int
		for _, s := range idxFlags {
			for _, f := range strings.Split(s, ",") {
				n, err := strconv.Atoi(strings.TrimSpace(f))
				if err != nil || n < 0 {
					return cli.Usagef("--gpu: %q is not a HIP index", f)
				}
				idx = append(idx, n)
			}
		}
		gpus, err := gpu.List("/")
		if err != nil {
			return err
		}
		cmds, err := tuning.SetPower(gpus, idx, watts)
		if err != nil {
			return err
		}
		res := map[string]any{"watts": watts, "applied": cmds == nil, "commands": cmds}
		return c.Emit(res, func(w io.Writer) {
			if cmds == nil {
				fmt.Fprintf(w, "power cap set to %.0f W\n", watts)
				return
			}
			fmt.Fprintln(w, "# not root: run these commands")
			for _, l := range cmds {
				fmt.Fprintln(w, l)
			}
		})
	}
	power.Subs = []*cli.Command{pstatus, pset}

	tune.Subs = []*cli.Command{top, power}
	cli.Register(tune)
}

// tunableOpState is `tune tunableop status`.
type tunableOpState struct {
	Venv    string `json:"venv"`
	ROCBLAS string `json:"rocblas"`
	SHA256  string `json:"sha256"`
	// Archs are the gfx targets of the GPUs the profile must fit: the host's
	// GPUs, or the ones HIP_VISIBLE_DEVICES lists. Empty: no GPU data.
	Archs   []string `json:"archs"`
	Profile string   `json:"profile"`
	RowsDir string   `json:"rows_dir"`
	PerUser string   `json:"per_user_file"`
}

func tunableOpStatus(venvFlag string) (tunableOpState, error) {
	cfg, err := config.Load()
	if err != nil {
		return tunableOpState{}, err
	}
	st := tunableOpState{Venv: cfg.ResolveVenv(venvFlag), PerUser: tuning.PerUserFile(), Archs: hostArchs()}
	st.ROCBLAS = rocm.FindROCBLAS("/", st.Venv)
	if st.ROCBLAS == "" {
		return st, nil
	}
	if st.SHA256, err = tuning.HashLib(st.ROCBLAS); err != nil {
		return st, err
	}
	p, err := tuning.Select(st.SHA256, "", st.Archs)
	if err != nil {
		return st, err
	}
	if p != nil {
		st.Profile, st.RowsDir = p.Name, tuning.RowsDir(p.Name)
	}
	return st, nil
}

// hostArchs returns the gfx targets of the host's GPUs, limited to the
// HIP_VISIBLE_DEVICES indices when that variable lists indices. It returns
// nil when the GPUs cannot be listed.
func hostArchs() []string {
	gpus, err := gpu.List("/")
	if err != nil {
		return nil
	}
	var idx []int
	if v := os.Getenv("HIP_VISIBLE_DEVICES"); v != "" {
		for _, f := range strings.Split(v, ",") {
			n, err := strconv.Atoi(strings.TrimSpace(f))
			if err != nil {
				idx = nil // not an index list (UUIDs): do not restrict
				break
			}
			idx = append(idx, n)
		}
	}
	if idx == nil {
		for _, g := range gpus {
			idx = append(idx, g.HIPIndex)
		}
	}
	return gpu.Archs(gpus, idx)
}

func short12(s string) string {
	if len(s) > tuning.SHAPrefix {
		return s[:tuning.SHAPrefix]
	}
	return s
}

func orNone(s string) string {
	if s == "" {
		return "(none)"
	}
	return s
}

func firstWords(s string, n int) string {
	f := strings.Fields(s)
	if len(f) <= n {
		return s
	}
	return strings.Join(f[:n], " ") + " ..."
}
