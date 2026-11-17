package placement

import (
	"encoding/json"
	"fmt"
)

// saved is what Save writes. VNodes is used only by ring and Size only by
// Maglev. Nodes is in the placement's own order, which matters for jump.
type saved struct {
	Algorithm string   `json:"algorithm"`
	VNodes    int      `json:"vnodes,omitempty"`
	Size      int      `json:"size,omitempty"`
	Nodes     []NodeID `json:"nodes"`
}

// Load rebuilds a placement from the bytes its Save returned.
func Load(data []byte) (Placement, error) {
	var s saved
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, fmt.Errorf("saved placement: %w", err)
	}

	var p Placement
	var err error
	switch s.Algorithm {
	case "modulo":
		p, err = NewModulo(s.Nodes)
	case "rendezvous":
		p, err = NewRendezvous(s.Nodes)
	case "ring":
		p, err = NewRing(s.Nodes, s.VNodes)
	case "jump":
		p, err = loadJump(s.Nodes)
	case "maglev":
		p, err = NewMaglev(s.Nodes, s.Size)
	default:
		return nil, fmt.Errorf("saved placement: unknown algorithm %q", s.Algorithm)
	}
	// On error p holds a nil pointer, which is still a non-nil Placement.
	if err != nil {
		return nil, err
	}
	return p, nil
}
