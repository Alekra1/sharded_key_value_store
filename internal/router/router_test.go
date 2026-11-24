package router

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"slices"
	"strings"
	"sync"
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
	path := filepath.Join(t.TempDir(), "state.json")
	if err := InitState(path, "rendezvous", nodes, 0, 0); err != nil {
		t.Fatal(err)
	}
	rt, err := Open(path)
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

func TestOpenRejectsMissingURL(t *testing.T) {
	p, err := placement.NewRendezvous([]placement.NodeID{"A", "B"})
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "state.json")
	if err := saveState(path, p, map[placement.NodeID]string{"A": "http://localhost:8001"}, false); err != nil {
		t.Fatal(err)
	}
	if _, err := Open(path); err == nil {
		t.Error("Open accepted a member without a URL")
	}
}

func TestWrongMethod(t *testing.T) {
	rt := threeEchoNodes(t)
	if rec := do(rt, "POST", "/kv?key=cat", ""); rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("POST: status %d, want 405", rec.Code)
	}
}

func statusNode(t *testing.T, status int, body string) string {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" || r.URL.Path != "/status" {
			t.Errorf("node got %s %s, want GET /status", r.Method, r.URL)
		}
		w.WriteHeader(status)
		io.WriteString(w, body)
	}))
	t.Cleanup(srv.Close)
	return srv.URL
}

func TestStatus(t *testing.T) {
	a := statusNode(t, http.StatusOK, `{"keys":3,"log_bytes":512}`)
	b := statusNode(t, http.StatusOK, `{"keys":5,"frozen":true,"broken":"disk full"}`)
	c := statusNode(t, http.StatusInternalServerError, "")
	down := echoNode(t, "D")
	rt := newRouter(t, map[placement.NodeID]string{"A": a, "B": b, "C": c, "D": down.URL})
	down.Close()

	rec := do(rt, "GET", "/status", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d, want 200 (body %q)", rec.Code, rec.Body.String())
	}
	var got protocol.ClusterStatus
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("reply %q: %v", rec.Body.String(), err)
	}

	saved, _ := rt.placement.Save()
	if string(got.Placement) != string(saved) {
		t.Errorf("placement %s, want %s", got.Placement, saved)
	}
	if len(got.Nodes) != 4 {
		t.Fatalf("got %d nodes, want 4: %+v", len(got.Nodes), got.Nodes)
	}
	if want := (protocol.NodeReport{Name: "A", URL: a, Keys: 3, LogBytes: 512}); got.Nodes[0] != want {
		t.Errorf("A: %+v, want %+v", got.Nodes[0], want)
	}
	if want := (protocol.NodeReport{Name: "B", URL: b, Keys: 5, Frozen: true, Broken: "disk full"}); got.Nodes[1] != want {
		t.Errorf("B: %+v, want %+v", got.Nodes[1], want)
	}
	if want := (protocol.NodeReport{Name: "C", URL: c, Unavailable: "node replied 500 Internal Server Error"}); got.Nodes[2] != want {
		t.Errorf("C: %+v, want %+v", got.Nodes[2], want)
	}
	if d := got.Nodes[3]; d.Name != "D" || d.Keys != 0 || d.Unavailable == "" {
		t.Errorf("D: %+v, want unavailable", d)
	}
}

func readClusterStatus(t *testing.T, rt *Router) protocol.ClusterStatus {
	t.Helper()
	rec := do(rt, "GET", "/status", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d, want 200 (body %q)", rec.Code, rec.Body.String())
	}
	var st protocol.ClusterStatus
	if err := json.Unmarshal(rec.Body.Bytes(), &st); err != nil {
		t.Fatalf("reply %q: %v", rec.Body.String(), err)
	}
	return st
}

func TestPause(t *testing.T) {
	rt := threeEchoNodes(t)
	if err := rt.Pause(); err != nil {
		t.Fatal(err)
	}
	if err := rt.Pause(); err != nil {
		t.Fatalf("second Pause: %v", err)
	}

	wantError(t, do(rt, "GET", "/kv?key=cat", ""), http.StatusServiceUnavailable)
	wantError(t, do(rt, "PUT", "/kv?key=cat", `{"value":"x"}`), http.StatusServiceUnavailable)
	wantError(t, do(rt, "DELETE", "/kv?key=cat", ""), http.StatusServiceUnavailable)
	if st := readClusterStatus(t, rt); !st.Maintenance || len(st.Nodes) != 3 {
		t.Errorf("status while paused: maintenance %v, %d nodes; want true, 3", st.Maintenance, len(st.Nodes))
	}

	if err := rt.Unpause(); err != nil {
		t.Fatal(err)
	}
	if err := rt.Unpause(); err != nil {
		t.Fatalf("second Unpause: %v", err)
	}
	readEcho(t, do(rt, "GET", "/kv?key=cat", ""))
	if st := readClusterStatus(t, rt); st.Maintenance {
		t.Error("status after Unpause still shows maintenance")
	}
}

func TestPauseWaitsForRequestsInProgress(t *testing.T) {
	arrived := make(chan struct{})
	release := make(chan struct{})
	slow := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(arrived)
		<-release
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(slow.Close)
	var once sync.Once
	// Cleanups run last first, so the handler is released before Close waits for it.
	t.Cleanup(func() { once.Do(func() { close(release) }) })

	rt := newRouter(t, map[placement.NodeID]string{"A": slow.URL})
	inProgress := make(chan *httptest.ResponseRecorder)
	go func() { inProgress <- do(rt, "PUT", "/kv?key=cat", `{"value":"x"}`) }()
	<-arrived

	paused := make(chan error)
	go func() { paused <- rt.Pause() }()
	select {
	case <-paused:
		t.Fatal("Pause returned while a request was in progress")
	case <-time.After(50 * time.Millisecond):
	}

	once.Do(func() { close(release) })
	if rec := <-inProgress; rec.Code != http.StatusNoContent {
		t.Errorf("request in progress: status %d, want 204 from the node", rec.Code)
	}
	if err := <-paused; err != nil {
		t.Fatal(err)
	}
	wantError(t, do(rt, "PUT", "/kv?key=cat", `{"value":"y"}`), http.StatusServiceUnavailable)
}

func TestPauseSurvivesRestart(t *testing.T) {
	rt := threeEchoNodes(t)
	if err := rt.Pause(); err != nil {
		t.Fatal(err)
	}

	restarted, err := Open(rt.statePath)
	if err != nil {
		t.Fatal(err)
	}
	if !restarted.Paused() {
		t.Fatal("router is not paused after a restart")
	}
	wantError(t, do(restarted, "GET", "/kv?key=cat", ""), http.StatusServiceUnavailable)

	if err := restarted.Unpause(); err != nil {
		t.Fatal(err)
	}
	again, err := Open(rt.statePath)
	if err != nil {
		t.Fatal(err)
	}
	if again.Paused() {
		t.Error("router is paused after Unpause and a restart")
	}
	if !slices.Equal(again.placement.Members(), rt.placement.Members()) {
		t.Errorf("members %v after restarts, want %v", again.placement.Members(), rt.placement.Members())
	}
}

func TestPauseSaveFails(t *testing.T) {
	rt := threeEchoNodes(t)
	rt.statePath = filepath.Join(t.TempDir(), "missing-dir", "state.json")
	if err := rt.Pause(); err == nil {
		t.Fatal("Pause succeeded without saving the mode")
	}
	if rt.Paused() {
		t.Error("router is paused although the mode was not saved")
	}
	readEcho(t, do(rt, "GET", "/kv?key=cat", ""))
}
