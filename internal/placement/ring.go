package placement

import (
	"errors"
	"fmt"
	"slices"
	"sort"
	"strconv"
)

type token struct {
	position uint64
	node     NodeID
}

type Ring struct {
	nodes  []NodeID
	vnodes int
	tokens []token
}

func NewRing(members []NodeID, vnodes int) (*Ring, error) {
	if len(members) == 0 {
		return nil, errors.New("ring placement requires at least one node")
	}
	if vnodes < 1 {
		return nil, fmt.Errorf("ring placement requires at least one virtual node per node, got %d", vnodes)
	}

	nodes := append([]NodeID(nil), members...)
	slices.Sort(nodes)
	for i := 1; i < len(nodes); i++ {
		if nodes[i] == nodes[i-1] {
			return nil, fmt.Errorf("duplicate node %q", nodes[i])
		}
	}

	r := &Ring{nodes: nodes, vnodes: vnodes}
	r.rebuild()
	return r, nil
}

func (r *Ring) rebuild() {
	tokens := make([]token, 0, len(r.nodes)*r.vnodes)
	for _, n := range r.nodes {
		for i := range r.vnodes {
			position := hash64(string(n), strconv.Itoa(i))
			tokens = append(tokens, token{position: position, node: n})
		}
	}

	sort.Slice(tokens, func(i, j int) bool {
		if tokens[i].position != tokens[j].position {
			return tokens[i].position < tokens[j].position
		}
		return tokens[i].node < tokens[j].node
	})
	r.tokens = tokens
}

func (r *Ring) Owner(key string) NodeID {
	return r.ownerAt(hash64(key))
}

func (r *Ring) ownerAt(position uint64) NodeID {
	i := sort.Search(len(r.tokens), func(i int) bool {
		return r.tokens[i].position >= position
	})
	if i == len(r.tokens) {
		i = 0
	}
	return r.tokens[i].node
}

func (r *Ring) Add(n NodeID) error {
	if slices.Contains(r.nodes, n) {
		return fmt.Errorf("duplicate node %q", n)
	}

	r.nodes = append(r.nodes, n)
	slices.Sort(r.nodes)
	r.rebuild()
	return nil
}

func (r *Ring) Remove(n NodeID) error {
	for i, node := range r.nodes {
		if node == n {
			if len(r.nodes) == 1 {
				return ErrLastNode
			}
			r.nodes = append(r.nodes[:i], r.nodes[i+1:]...)
			r.rebuild()
			return nil
		}
	}
	return ErrUnknownNode
}

func (r *Ring) Members() []NodeID {
	return append([]NodeID(nil), r.nodes...)
}

func (r *Ring) Save() ([]byte, error) {
	return nil, nil
}

var _ Placement = (*Ring)(nil)
