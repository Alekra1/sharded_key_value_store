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

// NodeStatus is a node's reply to GET /status.
type NodeStatus struct {
	Keys   int    `json:"keys"`
	Frozen bool   `json:"frozen,omitempty"`
	Broken string `json:"broken,omitempty"`
}

// ClusterStatus is the router's reply to GET /status. Placement holds the
// placement's saved form, the same bytes as in the router's state file.
type ClusterStatus struct {
	Placement   json.RawMessage `json:"placement"`
	Maintenance bool            `json:"maintenance,omitempty"`
	Nodes       []NodeReport    `json:"nodes"`
}

type NodeReport struct {
	Name   string `json:"name"`
	URL    string `json:"url"`
	Keys   int    `json:"keys"`
	Frozen bool   `json:"frozen,omitempty"`
	Broken string `json:"broken,omitempty"`
	// Set when the router could not get the node's status.
	Unavailable string `json:"unavailable,omitempty"`
}
