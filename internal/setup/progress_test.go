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

// TestDownloadProgress: the new sentence of the download (spec §6.1) is
// parsed into bytes, rate and file count.
func TestDownloadProgress(t *testing.T) {
	cases := []struct {
		name     string
		line     string
		want     DownloadProgress
		ok       bool
		fraction float64
		phase    string
	}{
		{
			name:     "complete line",
			line:     "model.download: 78% \u00b7 1.8 GB of 2.3 GB \u00b7 45.2 MB/s \u00b7 file 7 of 14",
			want:     DownloadProgress{Pct: 78, BytesDone: 1800000000, BytesTotal: 2300000000, RateBps: 45200000, FilesDone: 7, FilesTotal: 14},
			ok:       true,
			fraction: 1.8 / 2.3,
			phase:    PhaseDownload,
		},
		{
			name:     "no file count yet",
			line:     "model.download: 50% \u00b7 1.2 GB of 2.4 GB \u00b7 45.2 MB/s",
			want:     DownloadProgress{Pct: 50, BytesDone: 1200000000, BytesTotal: 2400000000, RateBps: 45200000},
			ok:       true,
			fraction: 0.5,
			phase:    PhaseDownload,
		},
		{
			name:     "no rate yet",
			line:     "model.download: 0% \u00b7 0 B of 2.3 GB \u00b7 file 1 of 14",
			want:     DownloadProgress{Pct: 0, BytesDone: 0, BytesTotal: 2300000000, FilesDone: 1, FilesTotal: 14},
			ok:       true,
			fraction: 0,
			phase:    PhaseDownload,
		},
		{
			name: "no numbers",
			line: "model.download: no numbers here",
			want: DownloadProgress{}, ok: false,
		},
		{
			name: "wrong prefix",
			line: "something else: 78% \u00b7 1.8 GB of 2.3 GB",
			want: DownloadProgress{}, ok: false,
		},
	}
	for _, c := range cases {
		got, ok := downloadProgress(c.line)
		if ok != c.ok {
			t.Errorf("%s: downloadProgress(%q) ok = %v, want %v", c.name, c.line, ok, c.ok)
			continue
		}
		if !ok {
			continue
		}
		if got != c.want {
			t.Errorf("%s: downloadProgress(%q) = %+v, want %+v", c.name, c.line, got, c.want)
		}
		p, ok := progressOf("model.download", c.line)
		if !ok {
			t.Errorf("%s: progressOf did not match", c.name)
			continue
		}
		if p.Phase != c.phase {
			t.Errorf("%s: phase = %q, want %q", c.name, p.Phase, c.phase)
		}
		if p.BytesDone != c.want.BytesDone || p.BytesTotal != c.want.BytesTotal {
			t.Errorf("%s: bytes = %d of %d, want %d of %d", c.name, p.BytesDone, p.BytesTotal, c.want.BytesDone, c.want.BytesTotal)
		}
		if p.RateBps != c.want.RateBps {
			t.Errorf("%s: rate = %d, want %d", c.name, p.RateBps, c.want.RateBps)
		}
		if p.FilesDone != c.want.FilesDone || p.FilesTotal != c.want.FilesTotal {
			t.Errorf("%s: files = %d of %d, want %d of %d", c.name, p.FilesDone, p.FilesTotal, c.want.FilesDone, c.want.FilesTotal)
		}
		if c.fraction > 0 && (p.Fraction < c.fraction-0.001 || p.Fraction > c.fraction+0.001) {
			t.Errorf("%s: fraction = %v, want %v", c.name, p.Fraction, c.fraction)
		}
	}
}
