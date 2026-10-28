package doctor

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestGroupGID(t *testing.T) {
	text := "root:x:0:\nvideo:x:44:op,alice\nrender:x:105:\nbroken:x:abc:\nshort\n"
	cases := []struct {
		name string
		gid  int
		ok   bool
	}{
		{"video", 44, true},
		{"render", 105, true},
		{"vid", 0, false},    // exact match only
		{"broken", 0, false}, // non-numeric gid
		{"missing", 0, false},
	}
	for _, c := range cases {
		gid, ok := groupGID(text, c.name)
		if gid != c.gid || ok != c.ok {
			t.Errorf("groupGID(%q) = %d, %v; want %d, %v", c.name, gid, ok, c.gid, c.ok)
		}
	}
}

func TestResolveGroupsFromRoot(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "etc"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "etc/group"), []byte("video:x:39:\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	// render is absent from the fixture root; a non-"/" root never runs getent.
	got := resolveGroups(context.Background(), root, deviceGroups)
	if len(got) != 1 || got["video"] != 39 {
		t.Errorf("resolveGroups = %v, want only video:39", got)
	}
}

func TestHasGID(t *testing.T) {
	if !hasGID([]int{1000, 44}, 44) || hasGID([]int{1000}, 44) || hasGID(nil, 0) {
		t.Error("hasGID")
	}
}
