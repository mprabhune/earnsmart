package handlers

import (
	"net/http"

	"earnsmart/internal/exporter"
	"earnsmart/internal/middleware"
)

// GET /api/v1/parent/export
//
// Returns a full JSON snapshot of the calling parent's family — profiles
// (with credential hashes), task definitions, task logs and ledger — for a
// one-time import into the local-first native app.
func (h *ParentHandler) ExportFamily(w http.ResponseWriter, r *http.Request) {
	claims, _ := middleware.GetClaims(r.Context())

	bundle, err := exporter.ExportFamily(h.DB, claims.FamilyID.String())
	if err != nil {
		RespondError(w, http.StatusInternalServerError, "Failed to build export: "+err.Error())
		return
	}

	w.Header().Set("Content-Disposition", `attachment; filename="earnsmart_export.json"`)
	RespondJSON(w, http.StatusOK, bundle)
}
