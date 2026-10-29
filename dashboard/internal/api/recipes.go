package api

import (
	"net/http"

	"github.com/opengfx1030/bernie/dashboard/internal/httpjson"
	"github.com/opengfx1030/bernie/internal/app"
	"github.com/opengfx1030/bernie/internal/serving/runner"
)

// GET /api/recipes — `bernie recipe list --all --json`.
func (h *handler) getRecipes(w http.ResponseWriter, r *http.Request) {
	l, err := app.RecipeList(true)
	if err != nil {
		httpjson.Error(w, http.StatusInternalServerError, err.Error())
		return
	}
	httpjson.Write(w, http.StatusOK, l)
}

// GET /api/recipes/{id}/plan?set=K=V&mode=M — `bernie recipe plan --json`.
func (h *handler) getPlan(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	p, err := app.Plan(r.Context(), runner.PrepareOptions{
		Recipe: r.PathValue("id"), Mode: q.Get("mode"), Name: q.Get("name"), Sets: q["set"],
	})
	if err != nil {
		httpjson.Error(w, http.StatusBadRequest, err.Error())
		return
	}
	httpjson.Write(w, http.StatusOK, p)
}
