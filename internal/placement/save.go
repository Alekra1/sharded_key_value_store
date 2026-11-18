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

	if s.Algorithm == "jump" {
		j, err := loadJump(s.Nodes)
		if err != nil {
			return nil, err
		}
		return j, nil
	}
	return New(s.Algorithm, s.Nodes, s.VNodes, s.Size)
}
