package httpapi

import (
	"encoding/json"
	"net/http"
)

// apiError is the JSON error envelope:
// {"error": {"code": "...", "message": "...", "details": {...}}}.
type apiError struct {
	Status  int    `json:"-"`
	Code    string `json:"code"`
	Message string `json:"message"`
	Details any    `json:"details,omitempty"`
}

func (e *apiError) Error() string { return e.Code + ": " + e.Message }

var (
	errInternal = &apiError{Status: http.StatusInternalServerError, Code: "internal", Message: "internal server error"}
	errNotFound = &apiError{Status: http.StatusNotFound, Code: "not_found", Message: "not found"}
)

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, _ *http.Request, e *apiError) {
	writeJSON(w, e.Status, map[string]*apiError{"error": e})
}
