package tuning

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/opengfx1030/bernie/internal/support/execx"
)

// Names of the generated systemd units.
const (
	PowerServiceUnit = "bernie-power.service"
	PowerPathUnit    = "bernie-power.path"
)

// PowerUnits holds the generated unit files and their contents.
type PowerUnits struct {
	ServiceFile string `json:"service_file"`
	PathFile    string `json:"path_file"`
	Service     string `json:"service"`
	Path        string `json:"path"`
}

// GeneratePowerUnits returns a hardened unit pair. binary is the absolute
// path of the installed bernie binary. desired is the desired-state file.
func GeneratePowerUnits(binary, desired, stateDir string) (PowerUnits, error) {
	var err error
	binary, err = filepath.Abs(binary)
	if err != nil {
		return PowerUnits{}, err
	}
	desired, err = filepath.Abs(desired)
	if err != nil {
		return PowerUnits{}, err
	}
	stateDir, err = filepath.Abs(stateDir)
	if err != nil {
		return PowerUnits{}, err
	}
	u := PowerUnits{
		ServiceFile: filepath.Join(stateDir, PowerServiceUnit),
		PathFile:    filepath.Join(stateDir, PowerPathUnit),
	}
	u.Service = powerServiceUnit(binary, desired)
	u.Path = powerPathUnit(desired)
	return u, nil
}

// Write writes both generated unit files under the state directory.
func (u PowerUnits) Write() error {
	if err := writeAtomic(u.ServiceFile, []byte(u.Service), 0o644); err != nil {
		return err
	}
	return writeAtomic(u.PathFile, []byte(u.Path), 0o644)
}

// InstallCommands returns the root commands that install the unit pair.
// bernie prints them; it never runs them.
func (u PowerUnits) InstallCommands() []string {
	return []string{
		fmt.Sprintf("sudo install -m644 %s /etc/systemd/system/%s && sudo install -m644 %s /etc/systemd/system/%s && sudo systemctl daemon-reload && sudo systemctl enable --now %s %s",
			shellQuote(u.ServiceFile), PowerServiceUnit,
			shellQuote(u.PathFile), PowerPathUnit,
			PowerServiceUnit, PowerPathUnit),
	}
}

// RemoveCommands returns the root commands that remove the unit pair.
func RemoveCommands() []string {
	return []string{
		"sudo systemctl disable --now " + PowerPathUnit + " " + PowerServiceUnit,
		"sudo rm -f /etc/systemd/system/" + PowerPathUnit + " /etc/systemd/system/" + PowerServiceUnit,
		"sudo systemctl daemon-reload",
	}
}

// PowerUnitState is the systemd state of one unit.
type PowerUnitState struct {
	Name          string `json:"name"`
	Available     bool   `json:"available"`
	Installed     bool   `json:"installed"`
	Enabled       bool   `json:"enabled"`
	Active        bool   `json:"active"`
	LoadState     string `json:"load_state"`
	UnitFileState string `json:"unit_file_state"`
	ActiveState   string `json:"active_state"`
	SubState      string `json:"sub_state"`
	Result        string `json:"result"`
	Detail        string `json:"detail,omitempty"`
}

// ReadPowerUnitState asks systemctl about one unit. A missing systemctl is a
// skip, not an error.
func ReadPowerUnitState(ctx context.Context, name string) PowerUnitState {
	out := PowerUnitState{Name: name}
	res, err := execx.Run(ctx, 3*time.Second, "systemctl", "show", name,
		"--property=LoadState,UnitFileState,ActiveState,SubState,Result", "--no-pager")
	if res.Missing {
		out.Detail = "systemctl is not installed"
		return out
	}
	if err != nil {
		out.Detail = err.Error()
		return out
	}
	for _, line := range strings.Split(res.Stdout, "\n") {
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		switch key {
		case "LoadState":
			out.LoadState = value
		case "UnitFileState":
			out.UnitFileState = value
		case "ActiveState":
			out.ActiveState = value
		case "SubState":
			out.SubState = value
		case "Result":
			out.Result = value
		}
	}
	out.Available = true
	out.Installed = out.LoadState == "loaded"
	out.Enabled = out.UnitFileState == "enabled" || out.UnitFileState == "enabled-runtime"
	out.Active = out.ActiveState == "active" || out.ActiveState == "activating" || out.ActiveState == "reloading"
	return out
}

func powerServiceUnit(binary, desired string) string {
	return fmt.Sprintf(`[Unit]
Description=Apply bernie GPU power caps
After=systemd-modules-load.service
ConditionPathExists=%s

[Service]
Type=oneshot
ExecStart=%s tune power apply --file %s
RemainAfterExit=no
UMask=0022
NoNewPrivileges=yes
PrivateTmp=yes
ProtectSystem=strict
ProtectHome=read-only
ReadOnlyPaths=%s
ReadWritePaths=/sys/class/drm /sys/class/hwmon
ProtectKernelTunables=yes
ProtectKernelModules=yes
ProtectKernelLogs=yes
ProtectControlGroups=yes
RestrictAddressFamilies=AF_UNIX
IPAddressDeny=any
LockPersonality=yes
MemoryDenyWriteExecute=yes
RestrictRealtime=yes
RestrictSUIDSGID=yes
SystemCallArchitectures=native
StandardOutput=journal
StandardError=journal

[Install]
WantedBy=multi-user.target
`, unitQuote(desired), unitQuote(binary), unitQuote(desired), unitQuote(desired))
}

func powerPathUnit(desired string) string {
	return fmt.Sprintf(`[Unit]
Description=Apply bernie GPU power caps when the desired state changes

[Path]
PathExists=%s
PathChanged=%s
Unit=%s

[Install]
WantedBy=multi-user.target
`, unitQuote(desired), unitQuote(desired), PowerServiceUnit)
}

// unitQuote quotes one systemd value.
func unitQuote(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	return `"` + strings.ReplaceAll(s, `"`, `\"`) + `"`
}

// shellQuote quotes one path for the root commands.
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// executablePath returns the absolute path of this binary.
func executablePath() (string, error) {
	p, err := os.Executable()
	if err != nil {
		return "", err
	}
	return filepath.Abs(p)
}
