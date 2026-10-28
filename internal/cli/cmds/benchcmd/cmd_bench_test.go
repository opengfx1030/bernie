package benchcmd

import (
	"strings"
	"testing"
)

func TestFingerprintLineShowsGPUScope(t *testing.T) {
	line := fingerprintLine(map[string]string{
		"host":      "gpu-box",
		"gpus":      "1x AMD Radeon PRO V620 (gfx1030)",
		"gpu_scope": "run",
		"run":       "flash",
	})
	for _, want := range []string{"host gpu-box", "gpus 1x AMD Radeon PRO V620 (gfx1030) (run)", "run flash"} {
		if !strings.Contains(line, want) {
			t.Errorf("fingerprint line %q lacks %q", line, want)
		}
	}
}
