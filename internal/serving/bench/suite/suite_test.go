package suite

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"
	"testing"
)

// TestBuiltInSuitesLoad checks that every built-in suite loads and says what
// it measures.
func TestBuiltInSuitesLoad(t *testing.T) {
	all, err := List()
	if err != nil {
		t.Fatal(err)
	}
	if len(all) < 2 {
		t.Fatalf("the built-in suites are %d, want at least the pr and the smoke suite", len(all))
	}
	byID := map[string]*Suite{}
	for _, s := range all {
		byID[s.ID] = s
	}
	pr, ok := byID["pr"]
	if !ok {
		t.Fatalf("the built-in suites %v lack the pr suite", ids(all))
	}
	if pr.Cells() != 6 {
		t.Errorf("the pr suite has %d cells, want 6", pr.Cells())
	}
	if pr.Recall != true {
		t.Error("the pr suite does not turn on the recall gate")
	}
	if pr.Compare.Band != DefaultBand || pr.Compare.Steady != DefaultSteady {
		t.Errorf("the pr suite thresholds are %+v", pr.Compare)
	}
	want := map[string][2]int{"1k": {1024, 512}, "16k": {16384, 1024}}
	for _, g := range pr.Groups {
		lens, ok := want[g.Name]
		if !ok {
			t.Errorf("the pr suite has the group %q", g.Name)
			continue
		}
		if g.InputLen != lens[0] || g.OutputLen != lens[1] {
			t.Errorf("the group %q is %d/%d tokens, want %d/%d", g.Name, g.InputLen, g.OutputLen, lens[0], lens[1])
		}
		if len(g.Concurrency) != 3 || g.Concurrency[0] != 1 || g.Concurrency[1] != 4 || g.Concurrency[2] != 8 {
			t.Errorf("the group %q has the levels %v, want 1,4,8", g.Name, g.Concurrency)
		}
		if g.Kind != KindThroughput {
			t.Errorf("the group %q has the kind %q", g.Name, g.Kind)
		}
	}
	if _, ok := byID["smoke"]; !ok {
		t.Errorf("the built-in suites %v lack the smoke suite", ids(all))
	}
}

// TestDigestIsTheHashOfTheCanonicalSuite checks that the digest of a suite
// is the sha256 of its canonical form, so two runs can be tied to the exact
// suite they used.
func TestDigestIsTheHashOfTheCanonicalSuite(t *testing.T) {
	s, err := Get("pr")
	if err != nil {
		t.Fatal(err)
	}
	b, err := json.Marshal(s)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(b)
	if s.Digest() != hex.EncodeToString(sum[:]) {
		t.Errorf("the digest %q is not the sha256 of the canonical suite", s.Digest())
	}
	other, err := Get("smoke")
	if err != nil {
		t.Fatal(err)
	}
	if other.Digest() == s.Digest() {
		t.Error("two different suites have the same digest")
	}
	again, err := Get("pr")
	if err != nil {
		t.Fatal(err)
	}
	if again.Digest() != s.Digest() {
		t.Error("the same suite has two digests")
	}
}

// TestLoadRejectsBadSuites checks that the loader refuses a suite it cannot
// run: an unknown key, a wrong schema, no group and a length of zero.
func TestLoadRejectsBadSuites(t *testing.T) {
	cases := map[string]struct {
		yaml string
		want string
	}{
		"unknown key": {
			yaml: "schema: 1\nid: x\nname: X\ngroups:\n  - name: g\n    input_len: 1\n    output_len: 1\n    concurrency: [1]\nnonsense: 3\n",
			want: "nonsense",
		},
		"wrong schema": {
			yaml: "schema: 2\nid: x\nname: X\ngroups:\n  - name: g\n    input_len: 1\n    output_len: 1\n    concurrency: [1]\n",
			want: "schema",
		},
		"no group": {
			yaml: "schema: 1\nid: x\nname: X\ngroups: []\n",
			want: "no groups",
		},
		"no input": {
			yaml: "schema: 1\nid: x\nname: X\ngroups:\n  - name: g\n    input_len: 0\n    output_len: 1\n    concurrency: [1]\n",
			want: "input_len",
		},
		"no concurrency": {
			yaml: "schema: 1\nid: x\nname: X\ngroups:\n  - name: g\n    input_len: 1\n    output_len: 1\n    concurrency: []\n",
			want: "concurrency",
		},
		"no id": {
			yaml: "schema: 1\nname: X\ngroups:\n  - name: g\n    input_len: 1\n    output_len: 1\n    concurrency: [1]\n",
			want: "id",
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := Load([]byte(tc.yaml))
			if err == nil {
				t.Fatal("the loader accepted a bad suite")
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("the error %q does not mention %q", err, tc.want)
			}
		})
	}
}

// TestLoadFillsDefaults checks that a suite that leaves values out gets the
// documented defaults.
func TestLoadFillsDefaults(t *testing.T) {
	s, err := Load([]byte("schema: 1\nid: x\nname: X\ngroups:\n  - name: g\n    input_len: 8\n    output_len: 4\n    concurrency: [2]\n"))
	if err != nil {
		t.Fatal(err)
	}
	if s.Reps != 1 || s.Compare.Band != DefaultBand || s.Compare.Steady != DefaultSteady {
		t.Errorf("the defaults are %+v", s)
	}
	if s.Groups[0].Reps != 1 || s.Groups[0].Kind != KindThroughput || s.Groups[0].Source.Kind != SourceSynthetic {
		t.Errorf("the group defaults are %+v", s.Groups[0])
	}
}

// TestGetUnknownSuite checks that an unknown suite is refused with a hint.
func TestGetUnknownSuite(t *testing.T) {
	_, err := Get("nope")
	if err == nil {
		t.Fatal("the loader found a suite that does not exist")
	}
	if !strings.Contains(err.Error(), "bench suites") {
		t.Errorf("the error %q does not point at the suite list", err)
	}
}

// ids names the ids of the suites.
func ids(all []*Suite) []string {
	out := make([]string, len(all))
	for i, s := range all {
		out[i] = s.ID
	}
	return out
}
