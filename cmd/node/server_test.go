package main

import (
	"encoding/json"
	"maps"
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

func readStatus(t *testing.T, rec *httptest.ResponseRecorder) protocol.NodeStatus {
	t.Helper()
	wantStatus(t, rec, http.StatusOK)
	if ct := rec.Result().Header.Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type %q, want application/json", ct)
	}
	var st protocol.NodeStatus
	if err := json.Unmarshal(rec.Body.Bytes(), &st); err != nil {
		t.Fatalf("reply %q: %v", rec.Body.String(), err)
	}
	return st
}

func TestStatus(t *testing.T) {
	s, st := newServer(t)
	empty := readStatus(t, do(s, "GET", "/status", ""))
	if empty.Keys != 0 || empty.Frozen || empty.Broken != "" || empty.LogBytes <= 0 {
		t.Errorf("empty node: %+v, want 0 keys, header bytes, not frozen/broken", empty)
	}

	wantStatus(t, do(s, "PUT", "/kv?key=cat", `{"value":"meow"}`), http.StatusNoContent)
	wantStatus(t, do(s, "PUT", "/kv?key=dog", `{"value":"woof"}`), http.StatusNoContent)
	after := readStatus(t, do(s, "GET", "/status", ""))
	if after.Keys != 2 || after.Frozen || after.Broken != "" || after.LogBytes <= empty.LogBytes {
		t.Errorf("after two puts: %+v, want 2 keys and larger log than %d", after, empty.LogBytes)
	}

	st.Close()
	wantError(t, do(s, "PUT", "/kv?key=fox", `{"value":"ring"}`), http.StatusInternalServerError)
	got := readStatus(t, do(s, "GET", "/status", ""))
	if got.Keys != 2 || !strings.HasPrefix(got.Broken, "store stopped") {
		t.Errorf("after a failed write: %+v, want 2 keys and the store's error", got)
	}
}

func TestFreeze(t *testing.T) {
	s, _ := newServer(t)
	wantStatus(t, do(s, "PUT", "/kv?key=cat", `{"value":"meow"}`), http.StatusNoContent)

	wantStatus(t, do(s, "POST", "/freeze", ""), http.StatusNoContent)
	wantError(t, do(s, "PUT", "/kv?key=dog", `{"value":"woof"}`), http.StatusServiceUnavailable)
	wantError(t, do(s, "DELETE", "/kv?key=cat", ""), http.StatusServiceUnavailable)
	wantValue(t, do(s, "GET", "/kv?key=cat", ""), "meow")
	if got := readStatus(t, do(s, "GET", "/status", "")); got.Keys != 1 || !got.Frozen || got.Broken != "" {
		t.Errorf("frozen node: %+v, want 1 key and frozen", got)
	}

	wantStatus(t, do(s, "POST", "/unfreeze", ""), http.StatusNoContent)
	wantStatus(t, do(s, "PUT", "/kv?key=dog", `{"value":"woof"}`), http.StatusNoContent)
	if got := readStatus(t, do(s, "GET", "/status", "")); got.Keys != 2 || got.Frozen || got.Broken != "" {
		t.Errorf("after unfreeze: %+v, want 2 keys and not frozen", got)
	}

	wantStatus(t, do(s, "GET", "/freeze", ""), http.StatusMethodNotAllowed)
}

func readRecords(t *testing.T, rec *httptest.ResponseRecorder) map[string]string {
	t.Helper()
	wantStatus(t, rec, http.StatusOK)
	var body protocol.Records
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("reply %q: %v", rec.Body.String(), err)
	}
	return body.Records
}

func TestRecords(t *testing.T) {
	s, _ := newServer(t)
	if got := readRecords(t, do(s, "GET", "/records", "")); got == nil || len(got) != 0 {
		t.Errorf("empty node: %#v, want an empty map", got)
	}
	wantStatus(t, do(s, "PUT", "/kv?key=cat", `{"value":"meow"}`), http.StatusNoContent)
	wantStatus(t, do(s, "POST", "/freeze", ""), http.StatusNoContent)

	wantStatus(t, do(s, "PUT", "/records", `{"records":{"dog":"woof","user 1+2":"a b","owl":"hoot"}}`), http.StatusNoContent)
	wantStatus(t, do(s, "DELETE", "/records", `{"keys":["owl","missing"]}`), http.StatusNoContent)

	want := map[string]string{"cat": "meow", "dog": "woof", "user 1+2": "a b"}
	if got := readRecords(t, do(s, "GET", "/records", "")); !maps.Equal(got, want) {
		t.Errorf("records %v, want %v", got, want)
	}
}

func TestRecordsBadRequests(t *testing.T) {
	s, _ := newServer(t)
	cases := []struct{ method, body string }{
		{"PUT", ""},
		{"PUT", "not json"},
		{"PUT", "{}"},
		{"PUT", `{"records":{"":"x"}}`},
		{"DELETE", ""},
		{"DELETE", "{}"},
		{"DELETE", `{"keys":"cat"}`},
	}
	for _, c := range cases {
		wantError(t, do(s, c.method, "/records", c.body), http.StatusBadRequest)
	}
	wantStatus(t, do(s, "POST", "/records", "{}"), http.StatusMethodNotAllowed)
}

func TestCompact(t *testing.T) {
	s, _ := newServer(t)
	for range 10 {
		wantStatus(t, do(s, "PUT", "/kv?key=cat", `{"value":"meow"}`), http.StatusNoContent)
	}

	wantError(t, do(s, "POST", "/compact", ""), http.StatusConflict)

	before := readStatus(t, do(s, "GET", "/status", "")).LogBytes
	wantStatus(t, do(s, "POST", "/freeze", ""), http.StatusNoContent)
	wantStatus(t, do(s, "POST", "/compact", ""), http.StatusNoContent)

	after := readStatus(t, do(s, "GET", "/status", "")).LogBytes
	if after >= before {
		t.Fatalf("log did not shrink: before %d, after %d", before, after)
	}
	wantValue(t, do(s, "GET", "/kv?key=cat", ""), "meow")
	wantStatus(t, do(s, "GET", "/compact", ""), http.StatusMethodNotAllowed)
}
