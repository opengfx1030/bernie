// Package factcache stores slow, stable facts of the host under the state
// directory.
//
// A fact is a small JSON document with a key. The cache lets a second read
// of the same host skip a probe that costs seconds (a Python that imports
// vLLM, a walk of the ROCm trees) without keeping the answer in memory
// only: the state directory survives a restart of bernie.
//
// A fact has a fingerprint: a short name for the host state it was read
// from. When the host state changes, the fingerprint changes and the entry
// is a miss, so a rebuilt venv or a new ROCm tree is probed again.
//
// A stale entry (older than its time to live) is served as it is and
// refreshed in the background: a reader never waits for a probe it does not
// need. One probe runs per key at a time.
//
// Nothing is ever written under /tmp: hosts reboot and /tmp is wiped.
package factcache

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/opengfx1030/bernie/internal/support/paths"
)

// Entry is one stored fact.
type Entry struct {
	Key         string          `json:"key"`
	Fingerprint string          `json:"fingerprint,omitempty"`
	StoredAt    time.Time       `json:"stored_at"`
	Value       json.RawMessage `json:"value"`
}

// Load reads the fact of key into value. It reports the age of the entry
// and whether the entry is fresh: younger than ttl and read from the same
// host state (the fingerprint matches). A missing, broken or foreign entry
// is a miss.
func Load(key, fingerprint string, ttl time.Duration, value any) (time.Duration, bool) {
	e, err := read(key)
	if err != nil {
		return 0, false
	}
	if e.Fingerprint != fingerprint {
		return 0, false
	}
	if err := json.Unmarshal(e.Value, value); err != nil {
		return 0, false
	}
	age := time.Since(e.StoredAt)
	if age < 0 {
		age = 0
	}
	return age, age < ttl
}

// Save stores value under key. The fingerprint names the host state the
// value was read from.
func Save(key, fingerprint string, value any) error {
	raw, err := json.Marshal(value)
	if err != nil {
		return err
	}
	return write(Entry{Key: key, Fingerprint: fingerprint, StoredAt: time.Now(), Value: raw})
}

// Get returns the fact of key. On a miss it calls produce, stores the
// result and returns it. On a stale entry (older than ttl) it returns the
// stored value at once and refreshes it in the background, so a reader
// never waits for a probe it does not need. The age of the entry comes
// back with the value; stale says that the value was older than the ttl.
//
// The producer must bound its own work: the cache does not stop a probe
// that hangs.
func Get[T any](ctx context.Context, key, fingerprint string, ttl time.Duration, produce func(context.Context) (T, error)) (T, time.Duration, bool, error) {
	var value T
	age, fresh := Load(key, fingerprint, ttl, &value)
	if fresh {
		return value, age, false, nil
	}
	if age > 0 {
		refresh(ctx, key, fingerprint, ttl, produce)
		return value, age, true, nil
	}
	mu := lockFor(key)
	mu.Lock()
	defer mu.Unlock()
	// Another reader may have filled the entry while this one waited.
	if age, fresh := Load(key, fingerprint, ttl, &value); fresh {
		return value, age, false, nil
	}
	value, err := produce(ctx)
	if err != nil {
		return value, 0, false, err
	}
	_ = Save(key, fingerprint, value)
	return value, 0, false, nil
}

// refresh probes a stale key in the background. The probe runs under the
// lock of the key, so one probe runs at a time.
func refresh[T any](ctx context.Context, key, fingerprint string, ttl time.Duration, produce func(context.Context) (T, error)) {
	go func() {
		mu := lockFor(key)
		mu.Lock()
		defer mu.Unlock()
		// The entry may have been refreshed while this one waited.
		var value T
		if _, fresh := Load(key, fingerprint, ttl, &value); fresh {
			return
		}
		value, err := produce(context.WithoutCancel(ctx))
		if err != nil {
			return
		}
		_ = Save(key, fingerprint, value)
	}()
}

// The locks of the keys: one producer per key at a time.
var (
	locksMu sync.Mutex
	locks   = map[string]*sync.Mutex{}
)

// lockFor returns the lock of a key.
func lockFor(key string) *sync.Mutex {
	locksMu.Lock()
	defer locksMu.Unlock()
	m, ok := locks[key]
	if !ok {
		m = &sync.Mutex{}
		locks[key] = m
	}
	return m
}

// dir is the directory the facts live in.
func dir() string { return filepath.Join(paths.State(), "facts") }

// file returns the path of the file of key.
func file(key string) string {
	name := sanitize(key)
	if len(name) > 80 {
		name = name[:80]
	}
	sum := sha256.Sum256([]byte(key))
	return filepath.Join(dir(), name+"-"+hex.EncodeToString(sum[:6])+".json")
}

// sanitize turns a key into a file name: letters, digits, '-', '_' and '.'.
func sanitize(key string) string {
	var b strings.Builder
	for _, r := range key {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_', r == '.':
			b.WriteRune(r)
		default:
			b.WriteRune('_')
		}
	}
	return b.String()
}

// read reads the entry of key.
func read(key string) (Entry, error) {
	b, err := os.ReadFile(file(key))
	if err != nil {
		return Entry{}, err
	}
	var e Entry
	if err := json.Unmarshal(b, &e); err != nil {
		return Entry{}, err
	}
	return e, nil
}

// write stores an entry. The file is written to a temporary name and
// renamed, so a reader never sees half a fact.
func write(e Entry) error {
	if _, err := paths.Ensure(dir()); err != nil {
		return err
	}
	b, err := json.Marshal(e)
	if err != nil {
		return err
	}
	path := file(e.Key)
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
