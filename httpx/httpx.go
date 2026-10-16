// Package httpx provides common HTTP and JSON utilities for proxy services.
package httpx

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
)

const maxDiscardBytes = 1 << 20

// WriteJSON sends a JSON response with Content-Type header and without HTML-escaping URLs.
func WriteJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	encoder := json.NewEncoder(w)
	encoder.SetEscapeHTML(false)
	_ = encoder.Encode(value)
}

// WriteError sends a standard JSON error object {"error": msg} with the given status code.
func WriteError(w http.ResponseWriter, status int, msg string) {
	WriteJSON(w, status, map[string]string{"error": msg})
}

// ReadBounded reads from reader up to limit bytes, returning an error if the data exceeds the limit.
func ReadBounded(reader io.Reader, limit int64) ([]byte, error) {
	if reader == nil {
		return nil, nil
	}
	limited := io.LimitReader(reader, limit+1)
	body, err := io.ReadAll(limited)
	if err != nil {
		return nil, err
	}
	if int64(len(body)) > limit {
		return nil, errors.New("request body exceeds size limit")
	}
	return body, nil
}

// BearerMatches checks in constant time whether the Authorization header contains the expected bearer token.
// Both expected and provided tokens are hashed with SHA-256 before comparison to prevent length timing leaks.
func BearerMatches(r *http.Request, key string) bool {
	if key == "" || r == nil {
		return false
	}
	auth := r.Header.Get("Authorization")
	parts := strings.SplitN(auth, " ", 2)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
		return false
	}
	expected := sha256.Sum256([]byte(key))
	provided := sha256.Sum256([]byte(parts[1]))
	return subtle.ConstantTimeCompare(expected[:], provided[:]) == 1
}

// RejectUnreadBody consumes up to maxDiscardBytes from an unread request body to enable connection reuse.
func RejectUnreadBody(w http.ResponseWriter, r *http.Request) {
	if r == nil || r.Body == nil {
		return
	}
	_, _ = io.CopyN(io.Discard, r.Body, maxDiscardBytes)
	_ = r.Body.Close()
}

// JSONString unmarshals a JSON string literal into a Go string. Returns empty string on error.
func JSONString(raw json.RawMessage) string {
	var s string
	_ = json.Unmarshal(raw, &s)
	return s
}

// SetJSON marshals value and stores it in object[key].
func SetJSON(object map[string]json.RawMessage, key string, value any) {
	if object == nil {
		return
	}
	encoded, err := json.Marshal(value)
	if err == nil {
		object[key] = encoded
	}
}
