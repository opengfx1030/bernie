package api

import (
	"net/http"

	"github.com/opengfx1030/bernie/dashboard/internal/httpjson"
	"github.com/opengfx1030/bernie/internal/glossary"
)

// GET /api/glossary — the words bernie uses that a reader who has never run
// vLLM may not know, with a plain meaning for each.
func (h *handler) getGlossary(w http.ResponseWriter, r *http.Request) {
	httpjson.Write(w, http.StatusOK, map[string]any{"words": glossary.Words})
}
