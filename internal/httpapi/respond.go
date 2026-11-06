package httpapi

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"

	"github.com/thomasbudiarjo/envily/internal/service"
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
	errInternal    = &apiError{Status: http.StatusInternalServerError, Code: "internal", Message: "internal server error"}
	errNotFound    = &apiError{Status: http.StatusNotFound, Code: "not_found", Message: "not found"}
	errBadJSON     = &apiError{Status: http.StatusBadRequest, Code: "bad_request", Message: "request body must be valid JSON"}
	errTooLarge    = &apiError{Status: http.StatusRequestEntityTooLarge, Code: "bad_request", Message: "request body is too large"}
	errCSRF        = &apiError{Status: http.StatusForbidden, Code: "forbidden", Message: "cross-site request rejected"}
	errAuthMissing = &apiError{Status: http.StatusUnauthorized, Code: "unauthorized", Message: "authentication required"}
)

func badRequest(msg string) *apiError {
	return &apiError{Status: http.StatusBadRequest, Code: "bad_request", Message: msg}
}

var statusByCode = map[string]int{
	service.CodeBadRequest:             http.StatusBadRequest,
	service.CodeValidationFailed:       http.StatusBadRequest,
	service.CodeUnauthorized:           http.StatusUnauthorized,
	service.CodeInvalidCredentials:     http.StatusUnauthorized,
	service.CodeForbidden:              http.StatusForbidden,
	service.CodeNotFound:               http.StatusNotFound,
	service.CodeConflict:               http.StatusConflict,
	service.CodeLastOwner:              http.StatusConflict,
	service.CodeRateLimited:            http.StatusTooManyRequests,
	service.CodeSetupRequired:          http.StatusForbidden,
	service.CodePasswordChangeRequired: http.StatusForbidden,
	service.CodeDecryptionFailed:       http.StatusInternalServerError,
	service.CodeInternal:               http.StatusInternalServerError,
}

// toAPIError maps any error to the response envelope. Unknown errors become
// "internal"; the caller logs them.
func toAPIError(err error) (*apiError, bool) {
	var ae *apiError
	if errors.As(err, &ae) {
		return ae, true
	}
	if se := service.AsError(err); se != nil {
		status, ok := statusByCode[se.Code]
		if !ok {
			status = http.StatusInternalServerError
		}
		e := &apiError{Status: status, Code: se.Code, Message: se.Message}
		if len(se.Details) > 0 {
			e.Details = se.Details
		}
		return e, se.Code != service.CodeDecryptionFailed
	}
	return errInternal, false
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, _ *http.Request, e *apiError) {
	writeJSON(w, e.Status, map[string]*apiError{"error": e})
}

// decode reads a JSON body into v. Unknown fields are rejected so typos in
// clients surface early.
func decode(r *http.Request, v any) error {
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			return errTooLarge
		}
		if errors.Is(err, io.EOF) {
			return badRequest("request body is required")
		}
		// The decoder error can quote input, which may be a secret value.
		return errBadJSON
	}
	if dec.More() {
		return errBadJSON
	}
	return nil
}

func itoa(n int) string { return strconv.Itoa(n) }

func parseInt(s string) (int, error) { return strconv.Atoi(s) }
