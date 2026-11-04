package http

import (
	"encoding/json"
	"net/http"

	"github.com/pauloeloy/databricks-governance-service/internal/domain"
)

// WriteProblem serializa um domain.ProblemError no padrão RFC 9457 (application/problem+json).
func WriteProblem(w http.ResponseWriter, r *http.Request, err error) {
	w.Header().Set("Content-Type", "application/problem+json; charset=utf-8")

	pErr, ok := err.(*domain.ProblemError)
	if !ok {
		pErr = domain.NewInternalError(err.Error())
	}

	if pErr.Instance == "" && r != nil {
		pErr.Instance = r.URL.Path
	}

	if reqID := w.Header().Get("X-Request-Id"); reqID != "" && pErr.RequestID == "" {
		pErr.RequestID = reqID
	}

	w.WriteHeader(pErr.Status)
	_ = json.NewEncoder(w).Encode(pErr)
}

// WriteJSON serializa respostas de sucesso com Content-Type application/json.
func WriteJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if data != nil {
		_ = json.NewEncoder(w).Encode(data)
	}
}
