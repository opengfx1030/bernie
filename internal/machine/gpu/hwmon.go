package gpu

// The hwmon node of a card and the labels of its sensors are set when the
// driver binds the card, and they do not change while the card stays
// bound. The sampler reads the cards of the host every second: finding the
// node and reading the labels again every time is a directory listing and
// a read per sensor, per card, per second, for nothing. They are found
// once and remembered.
//
// Every read of a value that moves (the temperature, the power) still goes
// to the card: only the names of the files are remembered, never a value.

import (
	"path/filepath"
	"strings"
	"sync"
)

var (
	hwmonMu sync.Mutex
	// hwmonNodes holds the hwmon node of every card, by device path.
	hwmonNodes = map[string]string{}
	// hwmonSensors holds the sensor of every label of every hwmon node.
	hwmonSensors = map[string]map[string]string{}
)

// hwmonDir returns the hwmon node of the card at dev, or "" when the card
// has none. A node that is found is remembered; a card without one is
// looked up again every time, because the driver may bind it later.
func hwmonDir(dev string) string {
	hwmonMu.Lock()
	defer hwmonMu.Unlock()
	if dir, ok := hwmonNodes[dev]; ok && dir != "" {
		return dir
	}
	dir := HwmonDir(dev)
	hwmonNodes[dev] = dir
	return dir
}

// tempSensor returns the file of the sensor that carries the label in the
// hwmon node, or "" when the node has no such sensor. The label of a
// sensor never changes: the mapping is remembered.
func tempSensor(hw, label string) string {
	hwmonMu.Lock()
	defer hwmonMu.Unlock()
	m, ok := hwmonSensors[hw]
	if !ok {
		m = map[string]string{}
		for _, l := range tempLabels(hw) {
			m[readString(l)] = filepath.Join(hw, strings.TrimSuffix(filepath.Base(l), "_label")+"_input")
		}
		hwmonSensors[hw] = m
	}
	return m[label]
}
