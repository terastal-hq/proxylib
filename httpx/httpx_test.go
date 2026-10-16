package httpx

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestWriteJSONAndError(t *testing.T) {
	w := httptest.NewRecorder()
	payload := map[string]string{"url": "https://example.com/api?a=1&b=2"}
	WriteJSON(w, http.StatusOK, payload)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	if ct := w.Header().Get("Content-Type"); ct != "application/json" {
		t.Fatalf("Content-Type = %q, want application/json", ct)
	}
	body := w.Body.String()
	if strings.Contains(body, "\\u0026") {
		t.Fatalf("URL was HTML-escaped: %s", body)
	}

	wErr := httptest.NewRecorder()
	WriteError(wErr, http.StatusBadRequest, "invalid request")
	if wErr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", wErr.Code)
	}
	if !strings.Contains(wErr.Body.String(), `"error":"invalid request"`) {
		t.Fatalf("unexpected error response: %s", wErr.Body.String())
	}
}

func TestReadBounded(t *testing.T) {
	reader := strings.NewReader("hello world")
	got, err := ReadBounded(reader, 20)
	if err != nil || string(got) != "hello world" {
		t.Fatalf("got %q, %v", got, err)
	}

	readerOverflow := strings.NewReader("12345678901")
	_, err = ReadBounded(readerOverflow, 10)
	if err == nil || !strings.Contains(err.Error(), "size limit") {
		t.Fatalf("expected size limit error, got %v", err)
	}

	gotNil, err := ReadBounded(nil, 10)
	if err != nil || gotNil != nil {
		t.Fatalf("expected nil for nil reader, got %v, %v", gotNil, err)
	}
}

func TestBearerMatches(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer secret-token")

	if !BearerMatches(req, "secret-token") {
		t.Fatal("expected match for valid bearer token")
	}
	if BearerMatches(req, "wrong-token") {
		t.Fatal("unexpected match for wrong token")
	}
	if BearerMatches(req, "") {
		t.Fatal("unexpected match for empty token")
	}

	reqNoAuth := httptest.NewRequest(http.MethodGet, "/", nil)
	if BearerMatches(reqNoAuth, "secret-token") {
		t.Fatal("unexpected match when no auth header")
	}

	reqBasic := httptest.NewRequest(http.MethodGet, "/", nil)
	reqBasic.Header.Set("Authorization", "Basic secret-token")
	if BearerMatches(reqBasic, "secret-token") {
		t.Fatal("unexpected match for basic auth")
	}

	if BearerMatches(nil, "secret-token") {
		t.Fatal("unexpected match for nil request")
	}
}

func TestRejectUnreadBody(t *testing.T) {
	body := io.NopCloser(bytes.NewReader([]byte("excess body")))
	req := httptest.NewRequest(http.MethodPost, "/", body)
	w := httptest.NewRecorder()
	RejectUnreadBody(w, req)

	// nil safety
	RejectUnreadBody(w, nil)
	reqNilBody := httptest.NewRequest(http.MethodGet, "/", nil)
	RejectUnreadBody(w, reqNilBody)
}

func TestJSONHelpers(t *testing.T) {
	raw := json.RawMessage(`"test-value"`)
	if got := JSONString(raw); got != "test-value" {
		t.Fatalf("JSONString = %q, want test-value", got)
	}
	if got := JSONString(json.RawMessage(`123`)); got != "" {
		t.Fatalf("JSONString on number = %q, want empty", got)
	}

	m := make(map[string]json.RawMessage)
	SetJSON(m, "foo", "bar")
	if string(m["foo"]) != `"bar"` {
		t.Fatalf("SetJSON = %s, want \"bar\"", m["foo"])
	}
	SetJSON(nil, "foo", "bar") // no panic
}
