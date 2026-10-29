package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"strconv"
	"strings"

	"github.com/opengfx1030/bernie/dashboard/internal/httpjson"
	"github.com/opengfx1030/bernie/internal/app"
)

// powerRequest is the body of PUT /api/power. gpu is "all" or a HIP index.
type powerRequest struct {
	GPU   json.RawMessage `json:"gpu"`
	Watts float64         `json:"watts"`
}

// GET /api/power — `bernie tune power service status --json`.
func (h *handler) getPower(w http.ResponseWriter, r *http.Request) {
	val, err := h.power.get(r.Context(), "", func(ctx context.Context) (any, error) {
		return app.ReadPowerServiceStatus(ctx)
	})
	if err != nil {
		httpjson.Error(w, http.StatusInternalServerError, err.Error())
		return
	}
	httpjson.Write(w, http.StatusOK, val)
}

// PUT /api/power — write one desired power cap.
func (h *handler) putPower(w http.ResponseWriter, r *http.Request) {
	var req powerRequest
	if !httpjson.Decode(w, r, &req) {
		return
	}
	if !validPowerRequest(w, req) {
		return
	}
	target, all, err := parsePowerTarget(req.GPU)
	if err != nil {
		httpjson.Error(w, http.StatusBadRequest, err.Error())
		return
	}
	status, err := app.ReadPowerServiceStatus(r.Context())
	if err != nil {
		httpjson.Error(w, http.StatusInternalServerError, err.Error())
		return
	}
	if !status.Installed {
		httpjson.Error(w, http.StatusConflict, "the power service is not installed; install it before changing a cap")
		return
	}
	watts := req.Watts
	if !all {
		// The domain clamps the wanted value to the range of the card and
		// names the cards that could not take it.
		if findPowerGPU(status, target) == nil {
			httpjson.Error(w, http.StatusBadRequest, fmt.Sprintf("GPU %d has no power cap", target))
			return
		}
	}
	var indexes []int
	if !all {
		indexes = []int{target}
	}
	result, err := app.PowerDesiredSet(indexes, watts)
	if err != nil {
		httpjson.Error(w, http.StatusInternalServerError, err.Error())
		return
	}
	// The cap changed: the next reader of the status sees it at once.
	h.power.drop("")
	httpjson.Write(w, http.StatusOK, result)
}

func validPowerRequest(w http.ResponseWriter, req powerRequest) bool {
	if len(req.GPU) == 0 {
		httpjson.Error(w, http.StatusBadRequest, "body field gpu is required")
		return false
	}
	if req.Watts <= 0 || math.IsNaN(req.Watts) || math.IsInf(req.Watts, 0) {
		httpjson.Error(w, http.StatusBadRequest, "body field watts must be a positive number")
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
