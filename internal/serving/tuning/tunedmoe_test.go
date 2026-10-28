package tuning

import (
	"os"
	"path/filepath"
	"testing"
	"testing/fstest"

	tunedmoe "github.com/opengfx1030/bernie/tuned-moe"
)

func TestTunedMoEArchLookup(t *testing.T) {
	fsys := fstest.MapFS{
		"gfx1030/E=1.json": {Data: []byte("a")},
		"gfx1100/E=2.json": {Data: []byte("b")},
		"gfx11/E=3.json":   {Data: []byte("c")},
		"stray.json":       {Data: []byte("d")},
		"README.md":        {Data: []byte("e")},
	}
	tests := []struct{ gfx, want string }{
		{"gfx1030", "gfx1030"},
		{"gfx1100", "gfx1100"},
		{"gfx1101", "gfx11"},
		{"gfx1031", ""},
		{"gfx90a", ""},
		{"", ""},
	}
	for _, tt := range tests {
		got, err := tunedMoEArch(fsys, tt.gfx)
		if err != nil || got != tt.want {
			t.Errorf("tunedMoEArch(%q) = %q, %v; want %q", tt.gfx, got, err, tt.want)
		}
	}
}

func TestMaterializeTunedMoEPerArch(t *testing.T) {
	fsys := fstest.MapFS{
		"gfx1030/E=1.json": {Data: []byte("a")},
		"gfx1100/E=2.json": {Data: []byte("b")},
	}
	root := t.TempDir()
	dir, err := materializeTunedMoE(fsys, "gfx1100", root)
	if err != nil {
		t.Fatal(err)
	}
	if dir != filepath.Join(root, "gfx1100") {
		t.Errorf("dir = %s", dir)
	}
	if b, _ := os.ReadFile(filepath.Join(dir, "E=2.json")); string(b) != "b" {
		t.Errorf("E=2.json = %q", b)
	}
	if _, err := os.Stat(filepath.Join(root, "gfx1030")); err == nil {
		t.Error("another architecture's folder was written")
	}
	// An architecture without a folder writes nothing and returns "".
	none := t.TempDir()
	if dir, err := materializeTunedMoE(fsys, "gfx1201", none); err != nil || dir != "" {
		t.Errorf("no folder: dir = %q, err = %v", dir, err)
	}
	if es, _ := os.ReadDir(none); len(es) != 0 {
		t.Errorf("wrote %d entries for an arch without a folder", len(es))
	}
}

func TestEmbeddedTunedMoE(t *testing.T) {
	dir, err := materializeTunedMoE(tunedmoe.FS, "gfx1030", t.TempDir())
	if err != nil || dir == "" {
		t.Fatalf("gfx1030: dir = %q, err = %v", dir, err)
	}
	if es, _ := os.ReadDir(dir); len(es) == 0 {
		t.Error("gfx1030 folder is empty")
	}
}
