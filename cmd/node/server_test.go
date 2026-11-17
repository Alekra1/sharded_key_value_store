package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Alekra1/sharded_key_value_store/internal/protocol"
	"github.com/Alekra1/sharded_key_value_store/internal/store"
)

func newServer(t *testing.T) (*server, *store.Store) {
	t.Helper()
	st, err := store.Create(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	return &server{store: st}, st
}

func do(s *server, method, target, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, target, strings.NewReader(body))
	rec := httptest.NewRecorder()
	s.routes().ServeHTTP(rec, req)
	return rec
}

func wantStatus(t *testing.T, rec *httptest.ResponseRecorder, want int) {
	t.Helper()
	if rec.Code != want {
		t.Fatalf("status %d, want %d (body %q)", rec.Code, want, rec.Body.String())
	}
}

func wantValue(t *testing.T, rec *httptest.ResponseRecorder, want string) {
	t.Helper()
	wantStatus(t, rec, http.StatusOK)
	var body protocol.Value
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("reply %q: %v", rec.Body.String(), err)
	}
	if body.Value == nil || *body.Value != want {
		t.Fatalf("reply %q, want value %q", rec.Body.String(), want)
	}
}

func wantError(t *testing.T, rec *httptest.ResponseRecorder, want int) {
	t.Helper()
	wantStatus(t, rec, want)
	if ct := rec.Result().Header.Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type %q, want application/json", ct)
	}
	var body protocol.Error
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil || body.Error == "" {
		t.Errorf("reply %q is not an error body", rec.Body.String())
	}
}

func TestPutGetDelete(t *testing.T) {
	s, _ := newServer(t)

	wantStatus(t, do(s, "PUT", "/kv?key=cat", `{"value":"meow"}`), http.StatusNoContent)
	wantValue(t, do(s, "GET", "/kv?key=cat", ""), "meow")
	wantStatus(t, do(s, "PUT", "/kv?key=cat", `{"value":"purr"}`), http.StatusNoContent)
	wantValue(t, do(s, "GET", "/kv?key=cat", ""), "purr")

	wantStatus(t, do(s, "DELETE", "/kv?key=cat", ""), http.StatusNoContent)
	wantError(t, do(s, "GET", "/kv?key=cat", ""), http.StatusNotFound)
	wantStatus(t, do(s, "DELETE", "/kv?key=cat", ""), http.StatusNoContent)
}

func TestEmptyValue(t *testing.T) {
	s, _ := newServer(t)
	wantStatus(t, do(s, "PUT", "/kv?key=empty", `{"value":""}`), http.StatusNoContent)
	wantValue(t, do(s, "GET", "/kv?key=empty", ""), "")
}

func TestKeyIsDecoded(t *testing.T) {
	s, st := newServer(t)
	wantStatus(t, do(s, "PUT", "/kv?key=user%201%2B2", `{"value":"Alice"}`), http.StatusNoContent)
	if v, ok := st.Get("user 1+2"); !ok || v != "Alice" {
		t.Fatalf("store has %q, %v under \"user 1+2\"; want Alice, true", v, ok)
	}
	wantValue(t, do(s, "GET", "/kv?key=user%201%2B2", ""), "Alice")
}

func TestBadRequests(t *testing.T) {
	longKey := strings.Repeat("k", 257)
	longValue := strings.Repeat("v", 65537)
	cases := []struct {
		name, method, target, body string
	}{
		{"get without key", "GET", "/kv", ""},
		{"put without key", "PUT", "/kv", `{"value":"x"}`},
		{"delete without key", "DELETE", "/kv", ""},
		{"put without value field", "PUT", "/kv?key=cat", `{}`},
		{"put with broken JSON", "PUT", "/kv?key=cat", `{"value":"x"`},
		{"put with empty body", "PUT", "/kv?key=cat", ""},
		{"put with long key", "PUT", "/kv?key=" + longKey, `{"value":"x"}`},
		{"put with long value", "PUT", "/kv?key=cat", `{"value":"` + longValue + `"}`},
		{"delete with long key", "DELETE", "/kv?key=" + longKey, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			s, _ := newServer(t)
			wantError(t, do(s, c.method, c.target, c.body), http.StatusBadRequest)
		})
	}
}

func TestBrokenStore(t *testing.T) {
	s, st := newServer(t)
	wantStatus(t, do(s, "PUT", "/kv?key=cat", `{"value":"meow"}`), http.StatusNoContent)
	// Closing the log makes the next write fail the way a disk error would.
	st.Close()

	wantError(t, do(s, "PUT", "/kv?key=dog", `{"value":"woof"}`), http.StatusInternalServerError)
	wantError(t, do(s, "DELETE", "/kv?key=cat", ""), http.StatusInternalServerError)
	wantValue(t, do(s, "GET", "/kv?key=cat", ""), "meow")
	wantError(t, do(s, "GET", "/kv?key=dog", ""), http.StatusNotFound)
}

func TestWrongMethod(t *testing.T) {
	s, _ := newServer(t)
	wantStatus(t, do(s, "POST", "/kv?key=cat", `{"value":"meow"}`), http.StatusMethodNotAllowed)
}
