package topo

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/opengfx1030/bernie/internal/machine/gpu"
	"github.com/opengfx1030/bernie/internal/support/fixture"
)

// checklist runs the readiness checklist on a fixture.
func checklist(t *testing.T, name string, o P2POptions) []CheckItem {
	t.Helper()
	root := fixture.Root(t, name)
	gpus, err := gpu.List(root)
	if err != nil {
		t.Fatal(err)
	}
	items, err := P2PChecklist(root, gpus, o)
	if err != nil {
		t.Fatal(err)
	}
	return items
}

func itemByID(t *testing.T, items []CheckItem, id string) CheckItem {
	t.Helper()
	for _, it := range items {
		if it.ID == id {
			return it
		}
	}
	t.Fatalf("checklist item %q missing; have %v", id, ids(items))
	return CheckItem{}
}

func ids(items []CheckItem) []string {
	var out []string
	for _, it := range items {
		out = append(out, it.ID)
	}
	return out
}

func TestP2PChecklistPar1(t *testing.T) {
	items := checklist(t, "par1-cs25", P2POptions{})
	want := map[string]P2PStatus{
		"kfd_links": P2POK, "large_bar": P2POK, "iommu": P2POK, "acs": P2POK,
		"link_speed": P2POK, "link_classes": P2PWarn,
		"bandwidth_test": P2PSkip, "recipe_env": P2PSkip,
	}
	if len(items) != len(want) {
		t.Fatalf("items = %v, want %d", ids(items), len(want))
	}
	for id, status := range want {
		if got := itemByID(t, items, id).Status; got != status {
			t.Errorf("%s = %s, want %s (%+v)", id, got, status, itemByID(t, items, id))
		}
	}
	links := itemByID(t, items, "kfd_links")
	if !strings.Contains(links.Detail, "45 of 45 GPU pairs") || !strings.Contains(links.Detail, "9 p2p_links") {
		t.Errorf("kfd_links detail = %q", links.Detail)
	}
	bar := itemByID(t, items, "large_bar")
	if !strings.Contains(bar.Detail, "32.0 GiB") || !strings.Contains(bar.Detail, "pci=realloc=off") {
		t.Errorf("large_bar detail = %q", bar.Detail)
	}
	classes := itemByID(t, items, "link_classes")
	if !strings.Contains(classes.Detail, "PHB") || !strings.Contains(classes.Detail, "PIX") {
		t.Errorf("link_classes detail = %q", classes.Detail)
	}
	if classes.Fix == "" || classes.Doc == "" {
		t.Errorf("link_classes has no fix or doc: %+v", classes)
	}
}

func TestP2PChecklistDegraded(t *testing.T) {
	items := checklist(t, "p2p-degraded-synthetic", P2POptions{})
	want := map[string]P2PStatus{
		"kfd_links": P2PWarn, "large_bar": P2PWarn, "iommu": P2PWarn, "acs": P2PWarn,
		"link_speed": P2PWarn, "link_classes": P2PWarn,
		"bandwidth_test": P2PSkip, "recipe_env": P2PSkip,
	}
	for id, status := range want {
		it := itemByID(t, items, id)
		if it.Status != status {
			t.Errorf("%s = %s, want %s (%+v)", id, it.Status, status, it)
		}
	}
	links := itemByID(t, items, "kfd_links")
	if !strings.Contains(links.Detail, "36 of 45 GPU pairs") || !strings.Contains(links.Detail, "GPU 4 has no p2p_link") {
		t.Errorf("kfd_links detail = %q", links.Detail)
	}
	bar := itemByID(t, items, "large_bar")
	if !strings.Contains(bar.Detail, "256 MiB") || strings.Contains(bar.Detail, "pci=realloc=off") {
		t.Errorf("large_bar detail = %q", bar.Detail)
	}
	link := itemByID(t, items, "link_speed")
	if !strings.Contains(link.Detail, "gen3 x8") || !strings.Contains(link.Detail, "gen4 x16") {
		t.Errorf("link_speed detail = %q", link.Detail)
	}
}

func TestP2PChecklistRecipeEnv(t *testing.T) {
	items := checklist(t, "par1-cs25", P2POptions{Recipe: RecipeEnv{
		ID:  "full",
		Env: map[string]string{"NCCL_P2P_LEVEL": "pix", "VLLM_RDNA_AR": "1", "OMP_NUM_THREADS": "4"},
	}})
	it := itemByID(t, items, "recipe_env")
	if it.Status != P2POK || !strings.Contains(it.Detail, "NCCL_P2P_LEVEL=pix") {
		t.Errorf("recipe_env = %+v", it)
	}
	if strings.Contains(it.Detail, "OMP_NUM_THREADS") {
		t.Errorf("recipe_env shows an unrelated variable: %q", it.Detail)
	}
}

// The VF BARs of an SR-IOV card come after the BARs of the device: 12 VFs of
// 32 GiB read as 384 GiB and must not pass for a large BAR.
func TestLargestBARIgnoresTheVFBARs(t *testing.T) {
	root := t.TempDir()
	dev := "sys/bus/pci/devices/0000:03:00.0"
	if err := os.MkdirAll(filepath.Join(root, dev), 0o755); err != nil {
		t.Fatal(err)
	}
	resource := strings.Join([]string{
		"0x00000000d0000000 0x00000000dfffffff 0x000000000014220c", // BAR0: 256 MiB
		"0x0000000000000000 0x0000000000000000 0x0000000000000000",
		"0x00000000e0000000 0x00000000e01fffff 0x000000000014220c",
		"0x0000000000000000 0x0000000000000000 0x0000000000000000",
		"0x000000000000e000 0x000000000000e0ff 0x0000000000040101",
		"0x00000000f0000000 0x00000000f00fffff 0x0000000000040200",
		"0x00000000f0100000 0x00000000f011ffff 0x0000000000046200", // ROM
		"0x0000039000000000 0x000003efffffffff 0x000000000014220c", // VF BAR0: 384 GiB
	}, "\n") + "\n"
	if err := os.WriteFile(filepath.Join(root, dev, "resource"), []byte(resource), 0o644); err != nil {
		t.Fatal(err)
	}
	got := largestBAR(root, gpu.GPU{SysfsPath: dev})
	if want := uint64(256 << 20); got != want {
		t.Errorf("largest BAR = %d, want %d", got, want)
	}
}

func TestLinkStatus(t *testing.T) {
	cases := []struct {
		linked, pairs int
		missingIO     bool
		want          P2PStatus
	}{
		{45, 45, false, P2POK},
		{36, 45, false, P2PWarn},
		{0, 45, false, P2PFail},
		{45, 45, true, P2PWarn},
	}
	for _, c := range cases {
		if got := linkStatus(c.linked, c.pairs, c.missingIO); got != c.want {
			t.Errorf("linkStatus(%d, %d, %v) = %s, want %s", c.linked, c.pairs, c.missingIO, got, c.want)
		}
	}
}

func TestHasPCIOpt(t *testing.T) {
	cmd := "ro iommu=pt pci=realloc=off,disable_acs_redir=pci:1000:c010"
	if !hasPCIOpt(cmd, "realloc=off") || !hasPCIOpt(cmd, "disable_acs_redir=pci:1000:c010") {
		t.Errorf("pci options not found in %q", cmd)
	}
	if hasPCIOpt("ro quiet", "realloc=off") {
		t.Error("realloc=off found in a command line without pci=")
	}
}
