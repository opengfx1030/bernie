package setup

import (
	"fmt"
	"strconv"
	"strings"
)

// Progress is the progress of a running step: the phase it is in, the
// sentence that says it and the numbers when the step counts them.
type Progress struct {
	// Phase names the phase of the step: clone, wheels, build, download
	// or verify.
	Phase string `json:"phase"`
	// Sentence is the one-line progress sentence (R20).
	Sentence string `json:"sentence"`
	// Fraction is the share of the step that is done, when it is known.
	Fraction float64 `json:"fraction,omitempty"`
	// Done and Total are the counted items of the step.
	Done  int `json:"done,omitempty"`
	Total int `json:"total,omitempty"`
	// BytesDone and BytesTotal are the bytes of a download.
	BytesDone  int64 `json:"bytes_done,omitempty"`
	BytesTotal int64 `json:"bytes_total,omitempty"`
}

// The phases of a step.
const (
	PhaseClone    = "clone"
	PhaseWheels   = "wheels"
	PhaseBuild    = "build"
	PhaseInstall  = "install"
	PhaseDownload = "download"
	PhaseVerify   = "verify"
)

// progressOf reads the progress out of one line of a step. It reports
// false when the line says nothing about the progress.
func progressOf(stepID, line string) (Progress, bool) {
	// The build of the fork: ninja counts its targets.
	if done, total, ok := ninjaFraction(line); ok {
		return Progress{
			Phase:    PhaseBuild,
			Sentence: fmt.Sprintf("building (%d/%d)", done, total),
			Fraction: float64(done) / float64(total),
			Done:     done,
			Total:    total,
		}, true
	}
	// The clone of the fork: git counts the objects it receives.
	if done, ok := gitFraction(line); ok {
		return Progress{
			Phase:    PhaseClone,
			Sentence: fmt.Sprintf("cloning (%d%%)", done),
			Fraction: float64(done) / 100,
			Done:     done,
			Total:    100,
		}, true
	}
	// The download of the model: the hub prints the bytes.
	if bytesDone, bytesTotal, ok := downloadBytes(line); ok {
		p := Progress{Phase: PhaseDownload, BytesDone: bytesDone, BytesTotal: bytesTotal}
		if bytesTotal > 0 {
			p.Fraction = float64(bytesDone) / float64(bytesTotal)
			p.Sentence = fmt.Sprintf("downloading (%.0f%%)", p.Fraction*100)
		} else {
			p.Sentence = "downloading"
		}
		return p, true
	}
	switch {
	case strings.HasPrefix(line, "Cloning into"):
		return Progress{Phase: PhaseClone, Sentence: "cloning the sources"}, true
	case strings.HasPrefix(line, "Collecting "):
		return Progress{Phase: PhaseWheels, Sentence: "collecting the packages"}, true
	case strings.HasPrefix(line, "Downloading "):
		return Progress{Phase: PhaseWheels, Sentence: "downloading the packages"}, true
	case strings.HasPrefix(line, "Installing collected packages"):
		return Progress{Phase: PhaseInstall, Sentence: "installing the packages"}, true
	case strings.HasPrefix(line, "Successfully installed"):
		return Progress{Phase: PhaseInstall, Sentence: "the packages are installed"}, true
	case strings.HasPrefix(line, "-- Configuring done"):
		return Progress{Phase: PhaseBuild, Sentence: "configuring the build"}, true
	case strings.HasPrefix(line, "-- Generating done"):
		return Progress{Phase: PhaseBuild, Sentence: "generating the build files"}, true
	case strings.HasPrefix(line, "-- Build files have been written to"):
		return Progress{Phase: PhaseBuild, Sentence: "the build files are written"}, true
	case strings.Contains(line, "BERNIE_JSON:"):
		return Progress{Phase: PhaseVerify, Sentence: "checking the venv"}, true
	}
	return Progress{}, false
}

// ninjaFraction reads the count of a ninja line: "[123/456] Building ...".
func ninjaFraction(line string) (done, total int, ok bool) {
	i := strings.Index(line, "[")
	if i < 0 {
		return 0, 0, false
	}
	j := strings.Index(line[i:], "]")
	if j < 0 {
		return 0, 0, false
	}
	parts := strings.Split(line[i+1:i+j], "/")
	if len(parts) != 2 {
		return 0, 0, false
	}
	done, err := strconv.Atoi(strings.TrimSpace(parts[0]))
	if err != nil {
		return 0, 0, false
	}
	total, err = strconv.Atoi(strings.TrimSpace(parts[1]))
	if err != nil || total <= 0 || done < 0 {
		return 0, 0, false
	}
	return done, total, true
}

// gitFraction reads the percentage of a git line: "Receiving objects: 45%".
func gitFraction(line string) (done int, ok bool) {
	i := strings.Index(line, "%")
	if i < 0 {
		return 0, false
	}
	head := strings.TrimSpace(line[:i])
	j := strings.LastIndex(head, " ")
	if j < 0 {
		return 0, false
	}
	n, err := strconv.Atoi(head[j+1:])
	if err != nil || n < 0 || n > 100 {
		return 0, false
	}
	return n, true
}

// downloadBytes reads the bytes of a download line:
// "model.download: 1.2 GB of 2.3 GB (52%) at 45.2 MB/s".
func downloadBytes(line string) (done, total int64, ok bool) {
	rest, found := strings.CutPrefix(line, "model.download: ")
	if !found {
		return 0, 0, false
	}
	at := strings.Split(rest, " at ")
	if len(at) != 2 {
		return 0, 0, false
	}
	sizes := strings.Split(at[0], " of ")
	if len(sizes) != 2 {
		return 0, 0, false
	}
	done, ok = parseSize(sizes[0])
	if !ok {
		return 0, 0, false
	}
	total, ok = parseSize(stripShare(sizes[1]))
	if !ok {
		return 0, 0, false
	}
	return done, total, true
}

// stripShare removes the share of a size: "2.3 GB (52%)" is "2.3 GB".
func stripShare(s string) string {
	if i := strings.Index(s, " ("); i >= 0 {
		return s[:i]
	}
	return s
}

// parseSize reads a size in bytes: "512 B", "45.2 MB". The units are
// decimal, like the ones the download prints.
func parseSize(s string) (int64, bool) {
	s = strings.TrimSpace(s)
	i := 0
	for i < len(s) && (s[i] == '.' || (s[i] >= '0' && s[i] <= '9')) {
		i++
	}
	if i == 0 {
		return 0, false
	}
	n, err := strconv.ParseFloat(s[:i], 64)
	if err != nil {
		return 0, false
	}
	var mult int64
	switch strings.TrimSpace(s[i:]) {
	case "B", "":
		mult = 1
	case "kB":
		mult = 1000
	case "MB":
		mult = 1000 * 1000
	case "GB":
		mult = 1000 * 1000 * 1000
	case "TB":
		mult = 1000 * 1000 * 1000 * 1000
	default:
		return 0, false
	}
	return int64(n * float64(mult)), true
}
