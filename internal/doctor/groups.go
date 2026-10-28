package doctor

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/opengfx1030/bernie/internal/support/execx"
)

// deviceGroups are the groups that grant access to /dev/kfd and /dev/dri.
var deviceGroups = []string{"video", "render"}

// getentTimeout bounds the getent fallback (LDAP/SSSD lookups can hang).
const getentTimeout = 3 * time.Second

// resolveGroups returns the gid of each named group. The binary is static
// (CGO_ENABLED=0), so os/user reads only /etc/group and cannot see groups that
// come from LDAP or SSSD: look in <root>/etc/group first and, on the live host
// only, fall back to `getent group <name>`. Groups found by neither are absent
// from the result.
func resolveGroups(ctx context.Context, root string, names []string) map[string]int {
	data, _ := os.ReadFile(filepath.Join(root, "etc/group"))
	out := map[string]int{}
	for _, name := range names {
		gid, ok := groupGID(string(data), name)
		if !ok && root == "/" {
			if res, err := execx.Run(ctx, getentTimeout, "getent", "group", name); err == nil && res.OK() {
				gid, ok = groupGID(execx.FirstLine(res.Stdout), name)
			}
		}
		if ok {
			out[name] = gid
		}
	}
	return out
}

// groupGID finds name in group(5) formatted text (name:passwd:gid:members).
func groupGID(text, name string) (int, bool) {
	for _, line := range strings.Split(text, "\n") {
		f := strings.Split(strings.TrimSpace(line), ":")
		if len(f) < 3 || f[0] != name {
			continue
		}
		if gid, err := strconv.Atoi(f[2]); err == nil {
			return gid, true
		}
	}
	return 0, false
}

func hasGID(gids []int, gid int) bool {
	for _, g := range gids {
		if g == gid {
			return true
		}
	}
	return false
}
