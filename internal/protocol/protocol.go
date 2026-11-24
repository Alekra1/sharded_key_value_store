package protocol

import "encoding/json"

// Value is the JSON body of a PUT request and of a successful GET reply.
type Value struct {
	// A pointer so a body without "value" is refused instead of storing "".
	Value *string `json:"value"`
}

// Error is the JSON body of every failed request.
type Error struct {
	Error string `json:"error"`
}

// Records is the body of GET and PUT /records on a node, which move data
// during maintenance.
type Records struct {
	Records map[string]string `json:"records"`
}

// Keys is the body of DELETE /records on a node.
type Keys struct {
	Keys []string `json:"keys"`
}

// AddNodeRequest is the JSON body of POST /nodes on the router.
type AddNodeRequest struct {
	ID  string `json:"id"`
	URL string `json:"url"`
}

// MoveReport records how much data a maintenance run moved and how long each
// stage took.
type MoveReport struct {
	Records   int   `json:"records"`
	Bytes     int   `json:"bytes"`
	FreezeMs  int64 `json:"freeze_ms"`
	CopyMs    int64 `json:"copy_ms"`
	VerifyMs  int64 `json:"verify_ms"`
	CleanupMs int64 `json:"cleanup_ms"`
	TotalMs   int64 `json:"total_ms"`
}

// NodeStatus is a node's reply to GET /status.
type NodeStatus struct {
	Keys     int    `json:"keys"`
	LogBytes int64  `json:"log_bytes,omitempty"`
	Frozen   bool   `json:"frozen,omitempty"`
	Broken   string `json:"broken,omitempty"`
}

// ClusterStatus is the router's reply to GET /status. Placement holds the
// placement's saved form, the same bytes as in the router's state file.
type ClusterStatus struct {
	Placement   json.RawMessage `json:"placement"`
	Maintenance bool            `json:"maintenance,omitempty"`
	Nodes       []NodeReport    `json:"nodes"`
}

type NodeReport struct {
	Name     string `json:"name"`
	URL      string `json:"url"`
	Keys     int    `json:"keys"`
	LogBytes int64  `json:"log_bytes,omitempty"`
	Frozen   bool   `json:"frozen,omitempty"`
	Broken   string `json:"broken,omitempty"`
	// Set when the router could not get the node's status.
	Unavailable string `json:"unavailable,omitempty"`
}
