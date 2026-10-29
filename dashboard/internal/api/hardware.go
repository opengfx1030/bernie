package api

import (
	"context"
	"errors"
	"io/fs"
	"net/http"

	"github.com/opengfx1030/bernie/dashboard/internal/httpjson"
	"github.com/opengfx1030/bernie/internal/app"
)

// GET /api/hw — `bernie hw --json`.
func (h *handler) getHW(w http.ResponseWriter, r *http.Request) {
	val, err := h.hw.get(r.Context(), "", func(ctx context.Context) (any, error) {
		rep, err := app.HW(ctx)
		if err == nil && rep.GPUErr != nil && !errors.Is(rep.GPUErr, fs.ErrNotExist) {
			err = rep.GPUErr // a host without KFD has no GPUs; other read errors are failures
		}
		if err != nil {
			return nil, err
		}
		return rep, nil
	})
	if err != nil {
		httpjson.Error(w, http.StatusInternalServerError, err.Error())
		return
	}
	httpjson.Write(w, http.StatusOK, val)
}
