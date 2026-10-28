# bernie package contracts

This page lists the API that packages agree on. Unexported helpers are free.
Fields and signatures here are a summary: the Go doc comments are the source.
The layers and the import rules are in [architecture.md](architecture.md).

Packages are grouped by role: `internal/machine` (what the host is),
`internal/serving` (what bernie runs) and `internal/support` (small helpers).

## machine/gpu

```go
type GPU struct { HIPIndex, KFDNode int; Name, GFX string; GFXTargetVersion int;
    PCI, DeviceID string; VRAMBytes uint64; RenderMinor, NUMANode int;
    Link Link; PowerCapW float64; SysfsPath string }
type Sample struct { HIPIndex int; BusyPct, TempC, PowerW, PowerCapW float64;
    VRAMUsed, VRAMTotal uint64 }
func List(root string) ([]GPU, error)
func Telemetry(root string, gpus []GPU) []Sample
func GFXName(targetVersion int) string

// Device tables (spec §4.4): internal/machine/gpu/devices/<generation>.yaml
type Arch struct { GFX, Generation, Family string; WaveSize int; BF16, HSAOverride,
    TritonBelow string; Integrated, FlashAttn bool }
type Device struct { Arch; ID, Name string; PowerCapAdviceW int }
func Lookup(deviceID string) (Device, bool)   // by PCI device id
func LookupArch(gfx string) (Arch, bool)
func CodeTargets(gfxs []string) []string      // arch -> arch whose code objects it runs
func ArchMatch(patterns []string, gfx string) bool   // full name or prefix
func Archs(gpus []GPU, idx []int) []string
```

`gpu` imports no other bernie package. No other package names a device.

## machine/host

```go
type Info struct { Hostname, Kernel, PkgManager, Virtualization string
    OS OSRelease; CPU CPU; RAMBytes uint64; User User; Engines []Engine
    SELinux, Cmdline string; ShmBytes uint64 }
func Probe(ctx context.Context, root string) (Info, error)   // root "/" in production
func Preferred(engines []Engine, name string) (Engine, bool)
func InstallCommand(pm, feature string) string               // package-manager fix text
```

## machine/rocm

```go
type Install struct { Root, Version, Kind string }   // Kind: opt|therock|distro|wheel
type Report struct { Installs []Install; Tools map[string]string; ROCBLAS, HIPVersion string; Agents []string }
type VenvReport struct { Python, Torch, HIP, VLLM string; DeviceCount, RDNAOps int; Err string }
func Probe(ctx context.Context, root string) Report
func ProbeVenv(ctx context.Context, venv string) VenvReport
func VenvLibPaths(venv string) []string          // ROCm wheel libs + torch/lib, in load order
func FindROCBLAS(root, venv string) string       // venv wheel first, then system
func CompareVersions(a, b string) int
```

## machine/topo

```go
type Pair struct { A, B int; Class string; Hops int; P2P bool; GBps float64 }
type Node struct { HIPIndex int; PCI string; Path []string; Switch, RootPort, Root string }
type Topology struct { Nodes []Node; Pairs []Pair; Bandwidth string }
func Discover(root string, gpus []gpu.GPU) (Topology, error)
func MeasureBandwidth(ctx context.Context, t *Topology, venv string) error
func BestGroup(t Topology, candidates []int, n int) (sel []int, reason string)
```

## doctor

```go
type Status string // ok|warn|fail|skip
type Check struct { ID, Category string; Status Status; Title, Detail, Fix, Doc string }
type Report struct { Summary map[Status]int; Checks []Check }
type Options struct { Root, Venv string; Categories []string; HFHome string; ValidatedArchs []string }
func Run(ctx context.Context, o Options) Report
func (Report) Failed() bool
```

`ValidatedArchs` comes from `recipe.ValidatedArchs`. `doctor` does not import
`recipe`. One `checks_<category>.go` file holds the checks of one category.

## logdoc

```go
type Rule struct { ID, Severity, Pattern, Cause, Fix, Doc string }
type Match struct { Rule; Line string; LineNo int }
func Load() ([]Rule, error)                       // embedded catalog.yaml, validated
func Scan(r io.Reader, rules []Rule) []Match      // first matching line per rule, streaming
func ScanFile(path string) ([]Match, error)
```

## setup

```go
type Step struct { ID, Title, Mode string; Root, Done bool; Commands []string; Why string }
type Options struct { Mode, Venv, Image, ModelRepo, SrcDir, HFHome string; GPUArchs []string }
func Plan(ctx context.Context, h host.Info, r rocm.Report, o Options) []Step
func Apply(ctx context.Context, steps []Step, only []string, log io.Writer) error
```

`Plan` takes the GPU targets from `GPUArchs` and asks the device tables for
code targets and the FlashAttention allowlist.

## serving/recipe

```go
type Set struct { Recipes map[string]*Recipe; Errors []LoadError }
func Load(extraDirs ...string) Set                 // layers: embedded, config, $BERNIE_RECIPES, extra
func (s Set) Resolve(id string) (*Recipe, error)   // extends merged
func Validate(r *Recipe) []string
type Fit struct { Status string; Reasons, Modes []string }   // fits|no|unknown
func CheckFit(r *Recipe, gpus []gpu.GPU, ramBytes uint64) Fit
func List(s Set, gpus []gpu.GPU, ramBytes uint64, all bool) Listing
func Best(l Listing) string                        // fit, then priority, then id
func ValidatedArchs(s Set) []string
type Plan struct { Recipe, Name, Mode, Entrypoint, Model, ServedName, Host string; Port int
    Env map[string]string; Args []string; GPUs []int; HIPDevices, GPUSelectReason string
    Image, ShmSize, TunableOpProfile, TunableOpRows string; PathInputs []string
    Mounts []Mount; Warnings []string; Exec *Exec }
type PlanOptions struct { Mode, Name string; Sets []string; GPUs []gpu.GPU; Topo *topo.Topology;
    TunedMoE func(gfx string) (string, error) }
func BuildPlan(r *Recipe, o PlanOptions) (*Plan, error)
```

## serving/tuning

```go
type Profile struct { Name, Dir, ROCBLAS, ROCm, Torch, LibSHA256, Status string;
    Arch []string; RowsPerRank int; Covers []string }
func Profiles() ([]Profile, error)                 // walks tunableop/<arch>/<profile>/profile.json
func HashLib(path string) (string, error)
func Select(libSHA, explicit string, archs []string) (*Profile, error)
func Materialize(p *Profile) (dir string, err error)       // <data>/tunableop/<name>
func Env(p *Profile, rowsDir string, tuning bool) map[string]string
func MaterializeTunedMoE(gfx string) (dir string, err error) // <data>/tuned-moe/<arch>, "" if none
type Power struct { HIPIndex int; CapW, MinW, MaxW float64; Path string }
func PowerStatus(gpus []gpu.GPU) []Power
func SetPower(gpus []gpu.GPU, idx []int, watts float64) (cmds []string, err error) // cmds when not root
```

## serving/runner

```go
type PrepareOptions struct { Recipe, Mode, Name, Venv, Engine string; Sets []string;
    Profile string; NoTunableOp, Tuning bool; RecipePaths []string }
func Prepare(ctx context.Context, o PrepareOptions) (*recipe.Plan, error) // config, recipe, GPUs, plan, host env, TunableOp, exec
type Record struct { Name, Recipe, Mode string; PID int; Container, Engine, Host string;
    Port int; StartedAt time.Time; Log, Image, ImageDigest string; Plan *recipe.Plan }
type Status struct { Record; State string }        // running|starting|exited
func Start(ctx context.Context, p *recipe.Plan, o StartOptions) (*Record, error)
func List(ctx context.Context) ([]Status, error)
func Stop(ctx context.Context, name string) error
func Logs(ctx context.Context, name string, follow bool, tail int, w io.Writer) error
func Get(name string) (*Record, error)             // one record; the error wraps os.ErrNotExist
func OpenLog(ctx context.Context, r Record, timeout time.Duration) (io.ReadCloser, error) // whole log: file, or `<engine> logs` under the execx timeout
func HostCommand(p *recipe.Plan, venv string) (argv []string, env []string, err error)
func ContainerCommand(p *recipe.Plan, eng host.Engine, rowsDir, hfCache string) []string
func Compose(p *recipe.Plan, rowsDir, hfCache string) ([]byte, error)
```

## serving/vllm and serving/bench

```go
// vllm
func New(baseURL string) *Client
func (c *Client) Health(ctx) error
func (c *Client) Models(ctx) ([]string, error)
func (c *Client) Tokenize(ctx, model, text string) (int, error)
func (c *Client) Metrics(ctx) (map[string]float64, error)   // flattened Prometheus samples
// bench
func Run(ctx context.Context, o Options, progress io.Writer) (*Result, error)
func Save(r *Result) (string, error); func List() ([]Result, error); func Get(id string) (*Result, error)
```

## flow

```go
type Summary struct { Host host.Info; GPUs []gpu.GPU; Doctor map[doctor.Status]int;
    Problems []doctor.Check; Best string; Fits []string; Runs []Run; Next []string }
func Snapshot(ctx context.Context, venv string) (*Summary, error)           // `status`
type UpOptions struct { Recipe string; Sets []string; Mode, Venv, Engine, Name string;
    Yes bool; Confirm func(prompt string) bool; Wait time.Duration }
type Blocked struct { Reason string; Commands []string }                      // exit code 3
func Up(ctx context.Context, o UpOptions, log io.Writer) (*UpResult, error)
func WaitHealthy(ctx context.Context, name string, limit time.Duration) error // *StartupExit when the run exits first
type Run struct { runner.Status; Findings []logdoc.Match }                    // Findings only for an exited run
type StartupExit struct { Name string; Findings []logdoc.Match }               // error of a run that exited before /health
func Runs(ctx context.Context) ([]Run, error)                                  // `ps`: runner.List plus findings
func Findings(ctx context.Context, r runner.Record) []logdoc.Match            // scan of an exited run's log, 2 s budget
```

`flow` never starts the dashboard. The front end does that after `Up` returns.

## support

```go
// paths: XDG roots with BERNIE_*_DIR overrides
func Config() string; func State() string; func Data() string; func Cache() string
func Ensure(dir string) (string, error)
// execx: bounded exec, always with a timeout
func Run(ctx context.Context, timeout time.Duration, name string, args ...string) (Result, error)
// config: <config>/config.yaml
func Load() (Config, error); func Save(c Config) error
func (c Config) ResolveVenv(flag string) string
// fixture: test host snapshots
func Root(tb testing.TB, name string) string
```

## cli and dashboard

`internal/cli` holds the command framework: `Register`, `Context` (flags,
`Emit` for `--json`), exit codes. A command is one `cmd_<name>.go` file in a
family package under `internal/cli/cmds/`. It calls `cli.Register` in
`init()`. `cmd/bernie/main.go` blank-imports each family.

`dashboard` serves the JSON API and the embedded `web/` of spec §11. A
handler calls the same packages as the matching command.
