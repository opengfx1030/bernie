package factcache

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// stateDir points the cache at a temporary directory.
func stateDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("BERNIE_STATE_DIR", dir)
	t.Setenv("XDG_STATE_HOME", "")
	return dir
}

func TestLoadMiss(t *testing.T) {
	stateDir(t)
	var value string
	age, fresh := Load("nothing", "", time.Minute, &value)
	if fresh || age != 0 {
		t.Errorf("an unknown key = age %v, fresh %v, want a miss", age, fresh)
	}
}

func TestSaveAndLoad(t *testing.T) {
	stateDir(t)
	if err := Save("greeting", "fp", "hello"); err != nil {
		t.Fatal(err)
	}
	var value string
	age, fresh := Load("greeting", "fp", time.Minute, &value)
	if !fresh {
		t.Errorf("a saved fact = age %v, fresh %v, want fresh", age, fresh)
	}
	if value != "hello" {
		t.Errorf("the fact = %q, want %q", value, "hello")
	}
}

func TestLoadExpired(t *testing.T) {
	stateDir(t)
	if err := Save("old", "fp", "value"); err != nil {
		t.Fatal(err)
	}
	// Age the entry: the time to live is over.
	e, err := read("old")
	if err != nil {
		t.Fatal(err)
	}
	e.StoredAt = time.Now().Add(-time.Hour)
	if err := write(e); err != nil {
		t.Fatal(err)
	}
	var value string
	age, fresh := Load("old", "fp", time.Minute, &value)
	if fresh {
		t.Error("an entry older than its time to live is fresh")
	}
	if age < time.Hour {
		t.Errorf("the age = %v, want more than an hour", age)
	}
	if value != "value" {
		t.Errorf("an expired entry = %q, want the stored value", value)
	}
}

func TestLoadOtherFingerprint(t *testing.T) {
	stateDir(t)
	if err := Save("fp", "one", "value"); err != nil {
		t.Fatal(err)
	}
	var value string
	if _, fresh := Load("fp", "two", time.Minute, &value); fresh {
		t.Error("a fact of another host state is fresh")
	}
}

func TestGetMissProduces(t *testing.T) {
	stateDir(t)
	calls := 0
	value, age, stale, err := Get(context.Background(), "made", "fp", time.Minute,
		func(context.Context) (string, error) {
			calls++
			return "produced", nil
		})
	if err != nil {
		t.Fatal(err)
	}
	if value != "produced" || stale || age != 0 {
		t.Errorf("a miss = %q, age %v, stale %v, want the produced value", value, age, stale)
	}
	var stored string
	if _, fresh := Load("made", "fp", time.Minute, &stored); !fresh || stored != "produced" {
		t.Errorf("the produced fact was not stored: %q, fresh %v", stored, fresh)
	}
}

func TestGetHitDoesNotProduce(t *testing.T) {
	stateDir(t)
	if err := Save("hit", "fp", "stored"); err != nil {
		t.Fatal(err)
	}
	calls := 0
	value, _, stale, err := Get(context.Background(), "hit", "fp", time.Minute,
		func(context.Context) (string, error) {
			calls++
			return "produced", nil
		})
	if err != nil {
		t.Fatal(err)
	}
	if value != "stored" || stale {
		t.Errorf("a hit = %q, stale %v, want the stored value", value, stale)
	}
	if calls != 0 {
		t.Errorf("the producer ran %d times on a hit, want 0", calls)
	}
}

func TestGetStaleServesAndRefreshes(t *testing.T) {
	stateDir(t)
	if err := Save("stale", "fp", "old"); err != nil {
		t.Fatal(err)
	}
	e, err := read("stale")
	if err != nil {
		t.Fatal(err)
	}
	e.StoredAt = time.Now().Add(-time.Hour)
	if err := write(e); err != nil {
		t.Fatal(err)
	}
	value, age, stale, err := Get(context.Background(), "stale", "fp", time.Minute,
		func(context.Context) (string, error) { return "new", nil })
	if err != nil {
		t.Fatal(err)
	}
	if value != "old" || !stale {
		t.Errorf("a stale entry = %q, stale %v, want the stored value", value, stale)
	}
	if age < time.Hour {
		t.Errorf("the age = %v, want more than an hour", age)
	}
	// The background refresh replaces the entry.
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		var stored string
		if _, fresh := Load("stale", "fp", time.Minute, &stored); fresh && stored == "new" {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Error("the stale entry was not refreshed")
}

func TestGetConcurrentMissProbesOnce(t *testing.T) {
	stateDir(t)
	var calls atomic.Int64
	produce := func(context.Context) (string, error) {
		calls.Add(1)
		time.Sleep(50 * time.Millisecond)
		return "produced", nil
	}
	var wg sync.WaitGroup
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			value, _, _, err := Get(context.Background(), "shared", "fp", time.Minute, produce)
			if err != nil || value != "produced" {
				t.Errorf("a concurrent read = %q, %v", value, err)
			}
		}()
	}
	wg.Wait()
	if n := calls.Load(); n != 1 {
		t.Errorf("the producer ran %d times, want 1", n)
	}
}

func TestSaveIsAtomic(t *testing.T) {
	dir := stateDir(t)
	if err := Save("atomic", "fp", "value"); err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(filepath.Join(dir, "facts"))
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if e.Name() == "atomic.tmp" {
			t.Error("the temporary file was left behind")
		}
	}
}
