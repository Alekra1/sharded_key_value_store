package router

import (
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

func writeError(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(protocol.Error{Error: msg})
}
