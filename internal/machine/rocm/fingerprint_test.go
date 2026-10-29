package rocm

import "testing"

// TestVenvFingerprintFollowsTheGPURestriction checks that the cached probe
// of a venv is thrown away when the operator limits the GPUs: the probe
// counts the GPUs that torch sees.
func TestVenvFingerprintFollowsTheGPURestriction(t *testing.T) {
	venv := t.TempDir()
	t.Setenv("HIP_VISIBLE_DEVICES", "")
	t.Setenv("ROCR_VISIBLE_DEVICES", "")
	plain := venvFingerprint(venv)
	t.Setenv("HIP_VISIBLE_DEVICES", "8")
	restricted := venvFingerprint(venv)
	if plain == restricted {
		t.Errorf("the fingerprint does not follow the GPU restriction: %s", plain)
	}
}
