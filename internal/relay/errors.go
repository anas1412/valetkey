package relay

import (
	"encoding/json"
	"net/http"
)

// API is the request shape the client speaks; errors are returned in the same shape.
type API int

const (
	OpenAI API = iota
	Anthropic
)

// writeError sends an error body in the client's API shape.
func writeError(w http.ResponseWriter, api API, status int, kind, message string) {
	var body any
	if api == Anthropic {
		body = map[string]any{"type": "error", "error": map[string]string{"type": kind, "message": message}}
	} else {
		body = map[string]any{"error": map[string]string{"message": message, "type": kind, "code": kind}}
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

// relayError is returned while preparing a request the client got wrong.
type relayError struct {
	status  int
	kind    string
	message string
}

func (e *relayError) Error() string { return e.message }

func badRequest(msg string) error {
	return &relayError{status: http.StatusBadRequest, kind: "invalid_request_error", message: msg}
}
