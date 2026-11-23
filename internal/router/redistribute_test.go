package router

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"net/http"
	"net/http/httptest"
	"slices"
	"sync"
	"testing"

	"github.com/Alekra1/sharded_key_value_store/internal/placement"
	"github.com/Alekra1/sharded_key_value_store/internal/protocol"
)

// memNode is an in-memory node server implementing the endpoints redistribution
// calls, plus /kv for end-to-end reads and writes in tests.
type memNode struct {
	mu        sync.Mutex
	data      map[string]string
	frozen    bool
	dropOnPut bool
	onFreeze  func()
	srv       *httptest.Server
}

func newMemNode(t *testing.T) *memNode {
	t.Helper()
	n := &memNode{data: make(map[string]string)}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /kv", n.get)
	mux.HandleFunc("PUT /kv", n.put)
	mux.HandleFunc("POST /freeze", n.freeze)
	mux.HandleFunc("POST /unfreeze", n.unfreeze)
	mux.HandleFunc("GET /records", n.records)
	mux.HandleFunc("PUT /records", n.putRecords)
	mux.HandleFunc("DELETE /records", n.deleteRecords)
	n.srv = httptest.NewServer(mux)
	t.Cleanup(n.srv.Close)
	return n
}

func (n *memNode) get(w http.ResponseWriter, r *http.Request) {
	n.mu.Lock()
	val, ok := n.data[r.URL.Query().Get("key")]
	n.mu.Unlock()
	if !ok {
		writeError(w, http.StatusNotFound, "key not found")
		return
	}
	writeJSON(w, http.StatusOK, protocol.Value{Value: &val})
}

func (n *memNode) put(w http.ResponseWriter, r *http.Request) {
	var body protocol.Value
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Value == nil {
		writeError(w, http.StatusBadRequest, "bad value")
		return
	}
	n.mu.Lock()
	defer n.mu.Unlock()
	if n.frozen {
		writeError(w, http.StatusServiceUnavailable, "frozen")
		return
	}
	n.data[r.URL.Query().Get("key")] = *body.Value
	w.WriteHeader(http.StatusNoContent)
}

func (n *memNode) freeze(w http.ResponseWriter, r *http.Request) {
	if n.onFreeze != nil {
		n.onFreeze()
	}
	n.mu.Lock()
	n.frozen = true
	n.mu.Unlock()
	w.WriteHeader(http.StatusNoContent)
}

func (n *memNode) unfreeze(w http.ResponseWriter, r *http.Request) {
	n.mu.Lock()
	n.frozen = false
	n.mu.Unlock()
	w.WriteHeader(http.StatusNoContent)
}

func (n *memNode) records(w http.ResponseWriter, r *http.Request) {
	n.mu.Lock()
	cp := maps.Clone(n.data)
	n.mu.Unlock()
	writeJSON(w, http.StatusOK, protocol.Records{Records: cp})
}

func (n *memNode) putRecords(w http.ResponseWriter, r *http.Request) {
	var body protocol.Records
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Records == nil {
		writeError(w, http.StatusBadRequest, "bad records")
		return
	}
	n.mu.Lock()
	if !n.dropOnPut {
		maps.Copy(n.data, body.Records)
	}
	n.mu.Unlock()
	w.WriteHeader(http.StatusNoContent)
}

func (n *memNode) deleteRecords(w http.ResponseWriter, r *http.Request) {
	var body protocol.Keys
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Keys == nil {
		writeError(w, http.StatusBadRequest, "bad keys")
		return
	}
	n.mu.Lock()
	for _, k := range body.Keys {
		delete(n.data, k)
	}
	n.mu.Unlock()
	w.WriteHeader(http.StatusNoContent)
}

func (n *memNode) snapshot() (map[string]string, bool) {
	n.mu.Lock()
	defer n.mu.Unlock()
	return maps.Clone(n.data), n.frozen
}

func putSampleData(t *testing.T, rt *Router, count int) map[string]string {
	t.Helper()
	want := make(map[string]string, count)
	for i := range count {
		key := fmt.Sprintf("key-%03d", i)
		val := fmt.Sprintf("val-%03d", i)
		want[key] = val
		rec := do(rt, "PUT", "/kv?key="+key, fmt.Sprintf(`{"value":%q}`, val))
		if rec.Code != http.StatusNoContent {
			t.Fatalf("PUT %s: status %d (body %q)", key, rec.Code, rec.Body.String())
		}
	}
	return want
}

func checkAllReadable(t *testing.T, rt *Router, want map[string]string) {
	t.Helper()
	for key, wantVal := range want {
		rec := do(rt, "GET", "/kv?key="+key, "")
		if rec.Code != http.StatusOK {
			t.Fatalf("GET %s: status %d (body %q)", key, rec.Code, rec.Body.String())
		}
		var body protocol.Value
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil || body.Value == nil || *body.Value != wantVal {
			t.Fatalf("GET %s: got %q, want %q", key, rec.Body.String(), wantVal)
		}
	}
}

func checkExactOwnership(t *testing.T, rt *Router, nodes map[placement.NodeID]*memNode, want map[string]string) {
	t.Helper()
	expected := make(map[placement.NodeID]map[string]string, len(nodes))
	for id := range nodes {
		expected[id] = make(map[string]string)
	}
	for key, val := range want {
		owner := rt.placement.Owner(key)
		expected[owner][key] = val
	}
	for id, node := range nodes {
		got, frozen := node.snapshot()
		if frozen {
			t.Errorf("node %s is still frozen", id)
		}
		if !maps.Equal(got, expected[id]) {
			t.Errorf("node %s has %d keys, want %d", id, len(got), len(expected[id]))
		}
	}
}

func TestAddNode(t *testing.T) {
	nodes := map[placement.NodeID]*memNode{
		"A": newMemNode(t),
		"B": newMemNode(t),
		"C": newMemNode(t),
	}
	rt := newRouter(t, map[placement.NodeID]string{
		"A": nodes["A"].srv.URL,
		"B": nodes["B"].srv.URL,
		"C": nodes["C"].srv.URL,
	})
	want := putSampleData(t, rt, 100)

	nodeD := newMemNode(t)
	nodes["D"] = nodeD

	report, err := rt.AddNode(context.Background(), "D", nodeD.srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	if rt.Paused() {
		t.Error("router is still paused after AddNode")
	}

	gotD, _ := nodeD.snapshot()
	if report.Records != len(gotD) || report.Records == 0 {
		t.Errorf("moved %d records, node D holds %d", report.Records, len(gotD))
	}
	wantBytes := 0
	for k, v := range gotD {
		wantBytes += len(k) + len(v)
	}
	if report.Bytes != wantBytes {
		t.Errorf("moved %d bytes, want %d", report.Bytes, wantBytes)
	}

	checkAllReadable(t, rt, want)
	checkExactOwnership(t, rt, nodes, want)

	restarted, err := Open(rt.statePath)
	if err != nil {
		t.Fatal(err)
	}
	if restarted.Paused() {
		t.Error("restarted router is paused")
	}
	if !slices.Equal(restarted.placement.Members(), []placement.NodeID{"A", "B", "C", "D"}) {
		t.Errorf("restarted members %v, want [A B C D]", restarted.placement.Members())
	}
	checkAllReadable(t, restarted, want)
}

func TestRemoveNode(t *testing.T) {
	nodes := map[placement.NodeID]*memNode{
		"A": newMemNode(t),
		"B": newMemNode(t),
		"C": newMemNode(t),
		"D": newMemNode(t),
	}
	rt := newRouter(t, map[placement.NodeID]string{
		"A": nodes["A"].srv.URL,
		"B": nodes["B"].srv.URL,
		"C": nodes["C"].srv.URL,
		"D": nodes["D"].srv.URL,
	})
	want := putSampleData(t, rt, 100)

	beforeB, _ := nodes["B"].snapshot()
	report, err := rt.RemoveNode(context.Background(), "B")
	if err != nil {
		t.Fatal(err)
	}
	if rt.Paused() {
		t.Error("router is still paused after RemoveNode")
	}
	if report.Records != len(beforeB) || report.Records == 0 {
		t.Errorf("moved %d records, B held %d", report.Records, len(beforeB))
	}

	checkAllReadable(t, rt, want)
	checkExactOwnership(t, rt, nodes, want)

	restarted, err := Open(rt.statePath)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(restarted.placement.Members(), []placement.NodeID{"A", "C", "D"}) {
		t.Errorf("restarted members %v, want [A C D]", restarted.placement.Members())
	}
	checkAllReadable(t, restarted, want)
}

func TestAddRemoveValidation(t *testing.T) {
	a := newMemNode(t)
	rt := newRouter(t, map[placement.NodeID]string{"A": a.srv.URL})

	if _, err := rt.AddNode(context.Background(), "", a.srv.URL); err == nil {
		t.Error("AddNode accepted empty id")
	}
	if _, err := rt.AddNode(context.Background(), "B", ""); err == nil {
		t.Error("AddNode accepted empty url")
	}
	if _, err := rt.AddNode(context.Background(), "A", a.srv.URL); err == nil {
		t.Error("AddNode accepted duplicate id")
	}
	if _, err := rt.RemoveNode(context.Background(), "Z"); !errors.Is(err, placement.ErrUnknownNode) {
		t.Errorf("RemoveNode unknown: got %v, want ErrUnknownNode", err)
	}
	if _, err := rt.RemoveNode(context.Background(), "A"); !errors.Is(err, placement.ErrLastNode) {
		t.Errorf("RemoveNode last: got %v, want ErrLastNode", err)
	}
	if rt.Paused() {
		t.Error("validation errors should not pause the router")
	}
}
