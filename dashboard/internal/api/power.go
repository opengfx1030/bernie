package dashboard

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"strconv"
	"strings"

	"github.com/opengfx1030/bernie/internal/app"
)

// powerRequest is the body of PUT /api/power. gpu is "all" or a HIP index.
type powerRequest struct {
	GPU   json.RawMessage `json:"gpu"`
	Watts float64         `json:"watts"`
}

// GET /api/power — `bernie tune power service status --json`.
func (s *server) getPower(w http.ResponseWriter, r *http.Request) {
	status, err := app.ReadPowerServiceStatus(r.Context())
	if err != nil {
		httpError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, status)
}

// PUT /api/power — write one desired power cap.
func (s *server) putPower(w http.ResponseWriter, r *http.Request) {
	var req powerRequest
	if !decodeBody(w, r, &req) {
		return
	}
	if !validPowerRequest(w, req) {
		return
	}
	target, all, err := parsePowerTarget(req.GPU)
	if err != nil {
		httpError(w, http.StatusBadRequest, err.Error())
		return
	}
	status, err := app.ReadPowerServiceStatus(r.Context())
	if err != nil {
		httpError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if !status.Installed {
		httpError(w, http.StatusConflict, "the power service is not installed; install it before changing a cap")
		return
	}
	watts := req.Watts
	if !all {
		row := findPowerGPU(status, target)
		if row == nil {
			httpError(w, http.StatusBadRequest, fmt.Sprintf("GPU %d has no power cap", target))
			return
		}
		watts = clampDashboardPower(watts, row.MinW, row.MaxW)
	}
	var indexes []int
	if !all {
		indexes = []int{target}
	}
	result, err := app.PowerDesiredSet(indexes, watts)
	if err != nil {
		httpError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func validPowerRequest(w http.ResponseWriter, req powerRequest) bool {
	if len(req.GPU) == 0 {
		httpError(w, http.StatusBadRequest, "body field gpu is required")
		return false
	}
	if req.Watts <= 0 || math.IsNaN(req.Watts) || math.IsInf(req.Watts, 0) {
		httpError(w, http.StatusBadRequest, "body field watts must be a positive number")
		return false
	}
	return true
}

func parsePowerTarget(raw json.RawMessage) (int, bool, error) {
	raw = json.RawMessage(strings.TrimSpace(string(raw)))
	if string(raw) == `"all"` {
		return 0, true, nil
	}
	var number int
	if err := json.Unmarshal(raw, &number); err == nil {
		if number < 0 {
			return 0, false, errors.New("body field gpu must be a HIP index or all")
		}
		return number, false, nil
	}
	var text string
	if err := json.Unmarshal(raw, &text); err != nil {
		return 0, false, errors.New("body field gpu must be a HIP index or all")
	}
	if text == "all" {
		return 0, true, nil
	}
	number, err := strconv.Atoi(text)
	if err != nil || number < 0 {
		return 0, false, errors.New("body field gpu must be a HIP index or all")
	}
	return number, false, nil
}

func findPowerGPU(status app.PowerServiceStatus, index int) *app.PowerGPUStatus {
	for i := range status.GPUs {
		if status.GPUs[i].HIPIndex == index {
			return &status.GPUs[i]
		}
	}
	return nil
}

func clampDashboardPower(watts, minW, maxW float64) float64 {
	if minW > 0 && watts < minW {
		watts = minW
	}
	if maxW > 0 && watts > maxW {
		watts = maxW
	}
	return watts
}
