package setup

import "testing"

// TestNinjaFraction: the count of a ninja line gives the fraction of the
// build that is done.
func TestNinjaFraction(t *testing.T) {
	cases := []struct {
		line  string
		done  int
		total int
		ok    bool
	}{
		{"[123/456] Building CUDA object vllm.o", 123, 456, true},
		{"[1/1] Generating vllm", 1, 1, true},
		{"  [10/20] Linking", 10, 20, true},
		{"[0/0] nothing", 0, 0, false},
		{"[abc/456] Building", 0, 0, false},
		{"[12] Building", 0, 0, false},
		{"Building without a count", 0, 0, false},
		{"", 0, 0, false},
	}
	for _, c := range cases {
		done, total, ok := ninjaFraction(c.line)
		if ok != c.ok || done != c.done || total != c.total {
			t.Errorf("ninjaFraction(%q) = %d/%d ok=%v, want %d/%d ok=%v",
				c.line, done, total, ok, c.done, c.total, c.ok)
		}
	}
}

// TestProgressOf: every kind of line gives the phase it belongs to.
func TestProgressOf(t *testing.T) {
	cases := []struct {
		name     string
		line     string
		phase    string
		fraction float64
		ok       bool
	}{
		{"ninja", "[123/456] Building CUDA object", PhaseBuild, 123.0 / 456.0, true},
		{"git", "Receiving objects:  45% (123/456)", PhaseClone, 0.45, true},
		{"clone", "Cloning into 'vllm-rdna'...", PhaseClone, 0, true},
		{"pip collect", "Collecting torch==2.12.0+rocm7.14.0", PhaseWheels, 0, true},
		{"pip download", "Downloading torch-2.12.0.whl (900 MiB)", PhaseWheels, 0, true},
		{"pip install", "Installing collected packages: torch", PhaseInstall, 0, true},
		{"pip done", "Successfully installed torch-2.12.0+rocm7.14.0", PhaseInstall, 0, true},
		{"cmake configure", "-- Configuring done", PhaseBuild, 0, true},
		{"cmake generate", "-- Generating done", PhaseBuild, 0, true},
		{"cmake write", "-- Build files have been written to: /src/build", PhaseBuild, 0, true},
		{"verify", "BERNIE_JSON:{\"python\": \"3.12.14\"}", PhaseVerify, 0, true},
		{"nothing", "a line that says nothing", "", 0, false},
	}
	for _, c := range cases {
		p, ok := progressOf("vllm.build", c.line)
		if ok != c.ok {
			t.Errorf("%s: progressOf(%q) ok = %v, want %v", c.name, c.line, ok, c.ok)
			continue
		}
		if !ok {
			continue
		}
		if p.Phase != c.phase {
			t.Errorf("%s: the phase is %q, want %q", c.name, p.Phase, c.phase)
		}
		if c.fraction != 0 && (p.Fraction < c.fraction-0.001 || p.Fraction > c.fraction+0.001) {
			t.Errorf("%s: the fraction is %v, want %v", c.name, p.Fraction, c.fraction)
		}
		if p.Sentence == "" {
			t.Errorf("%s: the progress has no sentence", c.name)
		}
	}
}

// TestDownloadBytes: the bytes of a download line give the fraction of the
// download that is done.
func TestDownloadBytes(t *testing.T) {
	done, total, ok := downloadBytes("model.download: 1.2 GB of 2.4 GB (50%) at 45.2 MB/s")
	if !ok {
		t.Fatal("the line of the download was not read")
	}
	if done != 1200000000 || total != 2400000000 {
		t.Errorf("the bytes are %d of %d, want 1200000000 of 2400000000", done, total)
	}
	if _, _, ok := downloadBytes("model.download: no numbers here"); ok {
		t.Error("a line without numbers must not be read as a download")
	}
}
