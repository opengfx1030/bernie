package gpu

import (
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

// dpmMHz matches the clock of one DPM state line ("2: 2570Mhz ").
var dpmMHz = regexp.MustCompile(`(\d+)\s*Mhz`)

// tempByLabel returns the temperature of the hwmon sensor with this label,
// in degrees Celsius. It is nil when the card has no such sensor.
func tempByLabel(hw, label string) *float64 {
	for _, l := range tempLabels(hw) {
		if readString(l) != label {
			continue
		}
		f := filepath.Join(hw, strings.TrimSuffix(filepath.Base(l), "_label")+"_input")
		v := readUint(f)
		if v == 0 {
			return nil
		}
		t := float64(v) / 1000
		return &t
	}
	return nil
}

// tempLabels lists the label files of an hwmon node.
func tempLabels(hw string) []string {
	l, _ := filepath.Glob(filepath.Join(hw, "temp*_label"))
	return l
}

// dpmClock reads the current clock of a pp_dpm_sclk or pp_dpm_mclk file. The
// line with the star marks the state the card runs in. It is nil when the
// card publishes no clock.
func dpmClock(path string) *int {
	body, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	for _, line := range strings.Split(string(body), "\n") {
		if !strings.Contains(line, "*") {
			continue
		}
		m := dpmMHz.FindStringSubmatch(line)
		if m == nil {
			return nil
		}
		v, err := strconv.Atoi(m[1])
		if err != nil {
			return nil
		}
		return &v
	}
	return nil
}
