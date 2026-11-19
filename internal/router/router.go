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
	"time"

	"github.com/Alekra1/sharded_key_value_store/internal/placement"
	"github.com/Alekra1/sharded_key_value_store/internal/protocol"
)

type Router struct {
	placement placement.Placement
	nodes     map[placement.NodeID]string
	client    *http.Client
}

// New takes the base URL of every member, for example
// "B": "http://localhost:8002".
func New(p placement.Placement, nodes map[placement.NodeID]string) (*Router, error) {
	for _, id := range p.Members() {
		if nodes[id] == "" {
			return nil, fmt.Errorf("no URL for node %q", id)
		}
	}
	return &Router{
		placement: p,
		nodes:     maps.Clone(nodes),
		client:    &http.Client{Timeout: 5 * time.Second},
	}, nil
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
	saved, err := rt.placement.Save()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	report := protocol.ClusterStatus{Placement: saved}
	for _, id := range rt.placement.Members() {
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
