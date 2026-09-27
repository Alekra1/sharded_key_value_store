package placement

import (
	"errors"
	"fmt"
	"slices"
)

type Modulo struct {
	nodes []NodeID
}

func NewModulo(members []NodeID) (*Modulo, error) {
	if len(members) == 0 {
		return nil, errors.New("modulo placement requires at least one node")
	}

	nodes := append([]NodeID(nil), members...)
	slices.Sort(nodes)
	for i := 1; i < len(nodes); i++ {
		if nodes[i] == nodes[i-1] {
			return nil, fmt.Errorf("duplicate node %q", nodes[i])
		}
	}

	return &Modulo{nodes: nodes}, nil
}

func (m *Modulo) Owner(key string) NodeID {
	return m.nodes[hash64(key)%uint64(len(m.nodes))]
}

func (m *Modulo) Add(n NodeID) error {
	if slices.Contains(m.nodes, n) {
		return fmt.Errorf("duplicate node %q", n)
	}

	m.nodes = append(m.nodes, n)
	slices.Sort(m.nodes)
	return nil
}

func (m *Modulo) Remove(n NodeID) error {
	for i, node := range m.nodes {
		if node == n {
			if len(m.nodes) == 1 {
				return ErrLastNode
			}
			m.nodes = append(m.nodes[:i], m.nodes[i+1:]...)
			return nil
		}
	}
	return ErrUnknownNode
}

func (m *Modulo) Members() []NodeID {
	return append([]NodeID(nil), m.nodes...)
}

// Just a placeholder for now
func (m *Modulo) Save() ([]byte, error) {
	return nil, nil
}

var _ Placement = (*Modulo)(nil)
