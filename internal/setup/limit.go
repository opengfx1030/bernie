package setup

import (
	"fmt"
	"sync/atomic"

	"github.com/opengfx1030/bernie/internal/support/config"
)

// downloadLimit holds the current bytes-per-second cap of a model
// download. atomic.Int64 so a live change from `config set` or the
// dashboard's `/api/download/limit` is visible to the next read of the
// downloader (R21: "changes take effect within a second").
var downloadLimit atomic.Int64

// CurrentDownloadLimit returns the bytes-per-second cap of a download:
// 0 means no cap. The downloader reads it on every write, so a change
// in the value reaches the next chunk immediately.
func CurrentDownloadLimit() int64 {
	return downloadLimit.Load()
}

// SetDownloadLimit changes the bytes-per-second cap of a download and
// returns the previous value. A negative value is rejected. The new
// limit takes effect within a second for every running download. The
// caller is also expected to save the value into the user config so the
// limit survives a restart.
func SetDownloadLimit(bps int64) (int64, error) {
	if bps < 0 {
		return 0, fmt.Errorf("download limit must not be negative, got %d", bps)
	}
	prev := downloadLimit.Swap(bps)
	return prev, nil
}

// LoadDownloadLimitFromConfig reads the limit from the user config file
// and applies it. A missing file or an invalid value is ignored: the
// in-memory value wins until a future SetDownloadLimit call changes it.
func LoadDownloadLimitFromConfig() {
	c, err := config.Load()
	if err != nil {
		return
	}
	downloadLimit.Store(c.DownloadLimit)
}

func init() {
	// Read the limit from disk at start-up so a value the operator set
	// with `bernie config set download.limit` is in force before the
	// first download begins.
	LoadDownloadLimitFromConfig()
}
