package placement

import (
	"errors"
	"fmt"
	"slices"
)

var errEmptyNodeName = errors.New("maglev placement does not allow an empty node name")

type Maglev struct {
	nodes []NodeID
	size  int
	table []NodeID
}

func NewMaglev(members []NodeID, size int) (*Maglev, error) {
	if len(members) == 0 {
		return nil, errors.New("maglev placement requires at least one node")
	}
	if !isPrime(size) {
		return nil, fmt.Errorf("maglev table size must be prime, got %d", size)
	}

	nodes := append([]NodeID(nil), members...)
	slices.Sort(nodes)
	if slices.Contains(nodes, "") {
		return nil, errEmptyNodeName
	}
	for i := 1; i < len(nodes); i++ {
		if nodes[i] == nodes[i-1] {
			return nil, fmt.Errorf("duplicate node %q", nodes[i])
		}
	}

	mg := &Maglev{nodes: nodes, size: size}
	mg.rebuild()
	return mg, nil
}

func (mg *Maglev) rebuild() {
	mg.table = maglevTable(mg.nodes, mg.size)
}

func (mg *Maglev) Owner(key string) NodeID {
	slot := hash64(key) % uint64(mg.size)
	return mg.table[slot]
}

func (mg *Maglev) Add(n NodeID) error {
	if n == "" {
		return errEmptyNodeName
	}
	if slices.Contains(mg.nodes, n) {
		return fmt.Errorf("duplicate node %q", n)
	}

	mg.nodes = append(mg.nodes, n)
	slices.Sort(mg.nodes)
	mg.rebuild()
	return nil
}

func (mg *Maglev) Remove(n NodeID) error {
	i := slices.Index(mg.nodes, n)
	if i < 0 {
		return ErrUnknownNode
	}
	if len(mg.nodes) == 1 {
		return ErrLastNode
	}

	mg.nodes = slices.Delete(mg.nodes, i, i+1)
	mg.rebuild()
	return nil
}

func (mg *Maglev) Members() []NodeID {
	return append([]NodeID(nil), mg.nodes...)
}

func (mg *Maglev) Save() ([]byte, error) {
	return nil, nil
}

var _ Placement = (*Maglev)(nil)

func isPrime(n int) bool {
	if n < 2 {
		return false
	}
	for d := 2; d*d <= n; d++ {
		if n%d == 0 {
			return false
		}
	}
	return true
}

type maglevNode struct {
	name NodeID
	next int
	skip int
}

func maglevTable(members []NodeID, m int) []NodeID {
	nodes := make([]maglevNode, len(members))
	for i, name := range members {
		nodes[i] = maglevNode{
			name: name,
			next: int(hash64(string(name), "offset") % uint64(m)),
			skip: int(hash64(string(name), "skip")%uint64(m-1)) + 1,
		}
	}

	table := make([]NodeID, m)
	filled := 0
	for filled < m {
		for i := range nodes {
			n := &nodes[i]
			slot := n.next
			for table[slot] != "" {
				slot = (slot + n.skip) % m
			}
			table[slot] = n.name
			n.next = (slot + n.skip) % m
			filled++
			if filled == m {
				break
			}
		}
	}
	return table
}
