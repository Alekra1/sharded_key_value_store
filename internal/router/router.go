package router

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"net"
	"net/http"
	"net/url"
	"sync"
	"time"

	"github.com/Alekra1/sharded_key_value_store/internal/placement"
	"github.com/Alekra1/sharded_key_value_store/internal/protocol"
)

type Router struct {
	statePath string
	placement placement.Placement
	nodes     map[placement.NodeID]string
	client    *http.Client

	// Every client request holds gate for reading while it runs. Pause takes
	// it for writing, so Pause waits for the requests in progress.
	gate   sync.RWMutex
	paused bool
}

// Open starts a router from the state file at path. Pause and Unpause save
// the mode to the same file.
func Open(path string) (*Router, error) {
	s, p, err := loadState(path)
	if err != nil {
		return nil, err
	}
	for _, id := range p.Members() {
		if s.URLs[id] == "" {
			return nil, fmt.Errorf("%s: no URL for node %q", path, id)
		}
	}
	return &Router{
		statePath: path,
		placement: p,
		nodes:     maps.Clone(s.URLs),
		client:    &http.Client{Timeout: 5 * time.Second},
		paused:    s.Maintenance,
	}, nil
}

// Pause refuses new client requests and returns when the requests in progress
// are finished. The mode is saved first, so a restarted router stays paused.
func (rt *Router) Pause() error {
	return rt.setPaused(true)
}

func (rt *Router) Unpause() error {
	return rt.setPaused(false)
}

func (rt *Router) setPaused(paused bool) error {
	rt.gate.Lock()
	defer rt.gate.Unlock()
	if rt.paused == paused {
		return nil
	}
	if err := saveState(rt.statePath, rt.placement, rt.nodes, paused); err != nil {
		return err
	}
	rt.paused = paused
	return nil
}

func (rt *Router) Paused() bool {
	rt.gate.RLock()
	defer rt.gate.RUnlock()
	return rt.paused
}

func (rt *Router) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /kv", rt.forward)
	mux.HandleFunc("PUT /kv", rt.forward)
	mux.HandleFunc("DELETE /kv", rt.forward)
	mux.HandleFunc("GET /status", rt.status)
	return mux
}

func (rt *Router) forward(w http.ResponseWriter, r *http.Request) {
	rt.gate.RLock()
	defer rt.gate.RUnlock()
	if rt.paused {
		writeError(w, http.StatusServiceUnavailable, "cluster is in maintenance; try again later")
		return
	}

	key := r.URL.Query().Get("key")
	if key == "" {
		writeError(w, http.StatusBadRequest, "key is empty")
		return
	}
	owner := rt.placement.Owner(key)
	// QueryEscape writes + as %2B. Left raw, the node would read it as a space.
	target := rt.nodes[owner] + "/kv?key=" + url.QueryEscape(key)

	req, err := http.NewRequestWithContext(r.Context(), r.Method, target, r.Body)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	resp, err := rt.client.Do(req)
	if err != nil {
		var netErr net.Error
		if errors.As(err, &netErr) && netErr.Timeout() {
			// The node may have applied a put or delete before the deadline,
			// so the client must not assume it failed.
			writeError(w, http.StatusGatewayTimeout, fmt.Sprintf("node %s did not answer in time; the outcome is unknown", owner))
			return
		}
		writeError(w, http.StatusBadGateway, fmt.Sprintf("node %s unavailable: %v", owner, err))
		return
	}
	defer resp.Body.Close()

	if ct := resp.Header.Get("Content-Type"); ct != "" {
		w.Header().Set("Content-Type", ct)
	}
	w.WriteHeader(resp.StatusCode)
	io.Copy(w, resp.Body)
}

// status asks the nodes one at a time, so a slow node delays the reply by up
// to the client timeout.
func (rt *Router) status(w http.ResponseWriter, r *http.Request) {
	// The nodes are asked after the gate is released, so a slow node does not
	// hold up Pause.
	rt.gate.RLock()
	saved, err := rt.placement.Save()
	members := rt.placement.Members()
	paused := rt.paused
	rt.gate.RUnlock()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	report := protocol.ClusterStatus{Placement: saved, Maintenance: paused}
	for _, id := range members {
		report.Nodes = append(report.Nodes, rt.nodeReport(r.Context(), id))
	}
	writeJSON(w, http.StatusOK, report)
}

func (rt *Router) nodeReport(ctx context.Context, id placement.NodeID) protocol.NodeReport {
	report := protocol.NodeReport{Name: string(id), URL: rt.nodes[id]}
	st, err := rt.askStatus(ctx, report.URL)
	if err != nil {
		report.Unavailable = err.Error()
		return report
	}
	report.Keys = st.Keys
	report.Frozen = st.Frozen
	report.Broken = st.Broken
	return report
}

func (rt *Router) askStatus(ctx context.Context, nodeURL string) (protocol.NodeStatus, error) {
	var st protocol.NodeStatus
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, nodeURL+"/status", nil)
	if err != nil {
		return st, err
	}
	resp, err := rt.client.Do(req)
	if err != nil {
		return st, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return st, fmt.Errorf("node replied %s", resp.Status)
	}
	if err := json.NewDecoder(resp.Body).Decode(&st); err != nil {
		return st, fmt.Errorf("bad status reply: %w", err)
	}
	return st, nil
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, protocol.Error{Error: msg})
}
