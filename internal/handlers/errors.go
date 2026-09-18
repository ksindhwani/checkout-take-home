package handlers

import (
	"encoding/json"
	"net/http"
)

type errorResponse struct {
	Error errorDetail `json:"error"`
}

type errorDetail struct {
	Code    string            `json:"code"`
	Message string            `json:"message,omitempty"`
	Fields  map[string]string `json:"fields,omitempty"`
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	writeRaw(w, status, mustMarshal(v))
}

func writeError(w http.ResponseWriter, status int, code, message string, fields map[string]string) {
	writeJSON(w, status, errorResponse{Error: errorDetail{Code: code, Message: message, Fields: fields}})
}

// writeRaw writes a pre-marshalled body, so idempotency can capture and
// replay the exact bytes later.
func writeRaw(w http.ResponseWriter, status int, body []byte) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = w.Write(body)
}

// mustMarshal marshals v, falling back to a fixed error body if that fails.
func mustMarshal(v any) []byte {
	body, err := json.Marshal(v)
	if err != nil {
		return []byte(`{"error":{"code":"internal_error","message":"failed to encode response"}}`)
	}
	return body
}
