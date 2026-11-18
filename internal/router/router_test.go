package router

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Alekra1/sharded_key_value_store/internal/placement"
	"github.com/Alekra1/sharded_key_value_store/internal/protocol"
)

// echo is what a fake node sends back, so a test can see which node got the
// request and what arrived there.
type echo struct {
	Node, Method, Key, Body string
}

func echoNode(t *testing.T, id string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(echo{id, r.Method, r.URL.Query().Get("key"), string(body)})
	}))
	t.Cleanup(srv.Close)
	return srv
}

func newRouter(t *testing.T, nodes map[placement.NodeID]string) *Router {
	t.Helper()
	ids := make([]placement.NodeID, 0, len(nodes))
	for id := range nodes {
		ids = append(ids, id)
	}
	p, err := placement.NewRendezvous(ids)
	if err != nil {
		t.Fatal(err)
	}
	rt, err := New(p, nodes)
	if err != nil {
		t.Fatal(err)
	}
	return rt
}

func threeEchoNodes(t *testing.T) *Router {
	return newRouter(t, map[placement.NodeID]string{
		"A": echoNode(t, "A").URL,
		"B": echoNode(t, "B").URL,
		"C": echoNode(t, "C").URL,
	})
}

// keyOwnedBy finds a key that the router sends to node id.
func keyOwnedBy(t *testing.T, rt *Router, id placement.NodeID) string {
	t.Helper()
	for i := range 1000 {
		key := fmt.Sprintf("key-%d", i)
		if rt.placement.Owner(key) == id {
			return key
		}
	}
	t.Fatalf("no key owned by %s", id)
	return ""
}

func do(rt *Router, method, target, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, target, strings.NewReader(body))
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, req)
	return rec
}

func readEcho(t *testing.T, rec *httptest.ResponseRecorder) echo {
	t.Helper()
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d, want 200 (body %q)", rec.Code, rec.Body.String())
	}
	var e echo
	if err := json.Unmarshal(rec.Body.Bytes(), &e); err != nil {
		t.Fatalf("reply %q: %v", rec.Body.String(), err)
	}
	return e
}

func wantError(t *testing.T, rec *httptest.ResponseRecorder, status int) {
	t.Helper()
	if rec.Code != status {
		t.Fatalf("status %d, want %d (body %q)", rec.Code, status, rec.Body.String())
	}
	var body protocol.Error
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil || body.Error == "" {
		t.Errorf("reply %q is not an error body", rec.Body.String())
	}
}

func TestForwardsToOwner(t *testing.T) {
	rt := threeEchoNodes(t)
	hits := map[string]int{}
	for i := range 300 {
		key := fmt.Sprintf("key-%d", i)
		e := readEcho(t, do(rt, "GET", "/kv?key="+key, ""))
		if want := string(rt.placement.Owner(key)); e.Node != want {
			t.Fatalf("%s went to %s, owner is %s", key, e.Node, want)
		}
		hits[e.Node]++
	}
	if len(hits) != 3 {
		t.Errorf("only nodes %v got requests", hits)
	}
}

func TestForwardsMethodKeyAndBody(t *testing.T) {
	rt := threeEchoNodes(t)
	cases := []struct{ method, body string }{
		{"PUT", `{"value":"Alice"}`},
		{"GET", ""},
		{"DELETE", ""},
	}
	for _, c := range cases {
		e := readEcho(t, do(rt, c.method, "/kv?key=user%201%2B2", c.body))
		if e.Method != c.method || e.Key != "user 1+2" || e.Body != c.body {
			t.Errorf("node got %+v, want %s of \"user 1+2\" with body %q", e, c.method, c.body)
		}
	}
}

func TestPassesNodeReplyThrough(t *testing.T) {
	node := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		io.WriteString(w, `{"error":"key not found"}`)
	}))
	t.Cleanup(node.Close)
	rt := newRouter(t, map[placement.NodeID]string{"A": node.URL})

	rec := do(rt, "GET", "/kv?key=cat", "")
	got := rec.Result()
	body, _ := io.ReadAll(got.Body)
	if got.StatusCode != http.StatusNotFound || string(body) != `{"error":"key not found"}` || got.Header.Get("Content-Type") != "application/json" {
		t.Errorf("got %d %q %q; want the node's 404 reply unchanged", got.StatusCode, body, got.Header.Get("Content-Type"))
	}
}

func TestEmptyKey(t *testing.T) {
	rt := threeEchoNodes(t)
	wantError(t, do(rt, "GET", "/kv", ""), http.StatusBadRequest)
}

func TestNodeDown(t *testing.T) {
	down := echoNode(t, "B")
	rt := newRouter(t, map[placement.NodeID]string{
		"A": echoNode(t, "A").URL,
		"B": down.URL,
	})
	down.Close()

	wantError(t, do(rt, "PUT", "/kv?key="+keyOwnedBy(t, rt, "B"), `{"value":"x"}`), http.StatusBadGateway)
	readEcho(t, do(rt, "GET", "/kv?key="+keyOwnedBy(t, rt, "A"), ""))
}

func TestNodeTimeout(t *testing.T) {
	release := make(chan struct{})
	slow := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-release
	}))
	t.Cleanup(slow.Close)
	// Cleanups run last first, so the handler is released before Close waits for it.
	t.Cleanup(func() { close(release) })

	rt := newRouter(t, map[placement.NodeID]string{"A": slow.URL})
	rt.client.Timeout = 50 * time.Millisecond
	wantError(t, do(rt, "PUT", "/kv?key=cat", `{"value":"x"}`), http.StatusGatewayTimeout)
}

func TestNewRejectsMissingURL(t *testing.T) {
	p, err := placement.NewRendezvous([]placement.NodeID{"A", "B"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := New(p, map[placement.NodeID]string{"A": "http://localhost:8001"}); err == nil {
		t.Error("New accepted a member without a URL")
	}
}

func TestWrongMethod(t *testing.T) {
	rt := threeEchoNodes(t)
	if rec := do(rt, "POST", "/kv?key=cat", ""); rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("POST: status %d, want 405", rec.Code)
	}
}
