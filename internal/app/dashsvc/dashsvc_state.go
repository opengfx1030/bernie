package dashsvc

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/opengfx1030/bernie/internal/support/paths"
)

// This file owns the state of the service on disk: the paths, the
// persistent token, the pidfile and the startup document. The service
// itself lives in dashsvc.go, the child in dashsvc_child.go and the
// systemd unit in dashsvc_systemd.go.

// File names in the state directory.
const (
	DirName     = "dashboard"
	TokenFile   = "token"
	PidFile     = "dash.pid"
	LogFile     = "dash.log"
	StartupFile = "startup.json"
	UnitFile    = "bernie-dash.service"
	UnitName    = "bernie-dash.service"
)

// dashNewToken is the dashboard token generator (a 128-bit hex string).
func dashNewToken() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		panic("dashsvc: crypto/rand: " + err.Error())
	}
	return hex.EncodeToString(b)
}

// Paths holds the file paths the service uses. The fields are absolute and
// always live under the XDG state dir (spec §12). Nothing is ever under
// /tmp.
type Paths struct {
	State   string // <state>/dashboard
	Token   string // <state>/dashboard/token
	Pid     string // <state>/dashboard/dash.pid
	Log     string // <state>/dashboard/dash.log
	Startup string // <state>/dashboard/startup.json
	Unit    string // <state>/dashboard/bernie-dash.service
}

// ServicePaths returns the file paths the service uses. The state dir is
// created lazily by EnsureDir; this function does not touch the disk.
func ServicePaths() Paths {
	base := filepath.Join(paths.State(), DirName)
	return Paths{
		State:   base,
		Token:   filepath.Join(base, TokenFile),
		Pid:     filepath.Join(base, PidFile),
		Log:     filepath.Join(base, LogFile),
		Startup: filepath.Join(base, StartupFile),
		Unit:    filepath.Join(base, UnitFile),
	}
}

// EnsureDir creates the state dir (mode 0700) if it does not exist and
// returns the Paths.
func EnsureDir() (Paths, error) {
	p := ServicePaths()
	if err := os.MkdirAll(p.State, 0o700); err != nil {
		return p, err
	}
	return p, nil
}

// ReadToken returns the persisted token, generating one and persisting it
// when the file is missing. The file is mode 0600.
func ReadToken() (string, Paths, error) {
	p, err := EnsureDir()
	if err != nil {
		return "", p, err
	}
	data, err := os.ReadFile(p.Token)
	if err == nil {
		tok := strings.TrimSpace(string(data))
		if tok != "" {
			return tok, p, nil
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", p, err
	}
	tok := dashNewToken()
	if err := os.WriteFile(p.Token, []byte(tok+"\n"), 0o600); err != nil {
		return "", p, err
	}
	return tok, p, nil
}

// WriteToken overwrites the persisted token.
func WriteToken(tok string) (Paths, error) {
	p, err := EnsureDir()
	if err != nil {
		return p, err
	}
	if err := os.WriteFile(p.Token, []byte(tok+"\n"), 0o600); err != nil {
		return p, err
	}
	return p, nil
}

// ReadableToken returns the persisted token when this process can read the
// token file. The file is mode 0600, so the owner (and root) read it and
// nobody else. The second answer is false when the file is missing or the
// process has no access: the caller then redacts the token.
func ReadableToken() (string, bool) {
	p := ServicePaths()
	data, err := os.ReadFile(p.Token)
	if err != nil {
		return "", false
	}
	tok := strings.TrimSpace(string(data))
	if tok == "" {
		return "", false
	}
	return tok, true
}

// IsAlive reports whether pid is a live process (signal 0 succeeds).
func IsAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	return syscall.Kill(pid, 0) == nil
}

// ReadPid reads the pidfile.
func ReadPid(p Paths) (int, error) {
	data, err := os.ReadFile(p.Pid)
	if err != nil {
		return 0, err
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		return 0, fmt.Errorf("dash pidfile: %w", err)
	}
	return pid, nil
}

// WritePid writes the PID file with the running dashboard's PID.
func WritePid(p Paths, pid int) error {
	return os.WriteFile(p.Pid, []byte(strconv.Itoa(pid)+"\n"), 0o644)
}

// RemovePid removes the PID file when present.
func RemovePid(p Paths) error {
	err := os.Remove(p.Pid)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}

// StartupInfo is the document the child writes when it starts listening.
// Status reads it back to print the URLs.
type StartupInfo struct {
	Listen    string    `json:"listen"`
	URL       string    `json:"url"`
	PID       int       `json:"pid"`
	StartedAt time.Time `json:"started_at"`
	Version   string    `json:"version"`
	Manager   string    `json:"manager"` // "process" or "systemd"
}

// WriteStartup writes the startup document (mode 0600).
func WriteStartup(p Paths, info StartupInfo) error {
	data, err := json.Marshal(info)
	if err != nil {
		return err
	}
	return os.WriteFile(p.Startup, data, 0o600)
}

// ReadStartup reads the startup document. Returns os.ErrNotExist when the
// child has not written one yet.
func ReadStartup(p Paths) (*StartupInfo, error) {
	data, err := os.ReadFile(p.Startup)
	if err != nil {
		return nil, err
	}
	var info StartupInfo
	if err := json.Unmarshal(data, &info); err != nil {
		return nil, fmt.Errorf("dash startup: %w", err)
	}
	return &info, nil
}

// RemoveStartup deletes the startup file when present.
func RemoveStartup(p Paths) error {
	err := os.Remove(p.Startup)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}
