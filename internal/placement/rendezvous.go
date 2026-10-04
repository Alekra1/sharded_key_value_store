package placement

import (
	"errors"
	"fmt"
	"slices"
)

type Rendezvous struct {
	nodes []NodeID
}

func NewRendezvous(members []NodeID) (*Rendezvous, error) {
	if len(members) == 0 {
		return nil, errors.New("rendezvous placement requires at least one node")
	}

	nodes := append([]NodeID(nil), members...)
	slices.Sort(nodes)
	for i := 1; i < len(nodes); i++ {
		if nodes[i] == nodes[i-1] {
			return nil, fmt.Errorf("duplicate node %q", nodes[i])
		}
	}

	return &Rendezvous{nodes: nodes}, nil
}

func (r *Rendezvous) Owner(key string) NodeID {
	owner := r.nodes[0]
	best := hash64(key, string(owner))

	for _, n := range r.nodes[1:] {
		if score := hash64(key, string(n)); score > best {
			owner, best = n, score
		}
	}
	return owner
}

func (r *Rendezvous) Add(n NodeID) error {
	if slices.Contains(r.nodes, n) {
		return fmt.Errorf("duplicate node %q", n)
	}

	r.nodes = append(r.nodes, n)
	slices.Sort(r.nodes)
	return nil
}

func (r *Rendezvous) Remove(n NodeID) error {
	for i, node := range r.nodes {
		if node == n {
			if len(r.nodes) == 1 {
				return ErrLastNode
			}
			r.nodes = append(r.nodes[:i], r.nodes[i+1:]...)
			return nil
		}
	}
	return ErrUnknownNode
}

func (r *Rendezvous) Members() []NodeID {
	return append([]NodeID(nil), r.nodes...)
}

// Just a placeholder for now
func (r *Rendezvous) Save() ([]byte, error) {
	return nil, nil
}

var _ Placement = (*Rendezvous)(nil)
