// Package hostroot resolves the root of the host tree bernie reads when it
// inspects a machine. It is "/" on a real host. BERNIE_ROOT points it at
// another tree, a captured host snapshot, so the CLI, the dashboard and the
// MCP server can show a host that is not this one. bernie never writes
// there.
package hostroot

import (
	"os"
	"path/filepath"
	"strings"
)

// EnvVar names the environment variable that holds the root.
const EnvVar = "BERNIE_ROOT"

// Root is the root of the host tree bernie reads. It is "/" unless
// BERNIE_ROOT names another tree.
func Root() string {
	v := strings.TrimSpace(os.Getenv(EnvVar))
	if v == "" {
		return "/"
	}
	return filepath.Clean(v)
}

// Set points bernie at another host tree. An empty value means the real
// host again.
func Set(root string) {
	if strings.TrimSpace(root) == "" {
		_ = os.Unsetenv(EnvVar)
		return
	}
	_ = os.Setenv(EnvVar, filepath.Clean(root))
}
