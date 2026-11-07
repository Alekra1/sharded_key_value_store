package placement

import (
	"errors"
	"fmt"
	"slices"
)

type Jump struct {
	nodes []NodeID
}

func NewJump(members []NodeID) (*Jump, error) {
	if len(members) == 0 {
		return nil, errors.New("jump placement requires at least one node")
	}

	nodes := append([]NodeID(nil), members...)
	slices.Sort(nodes)
	for i := 1; i < len(nodes); i++ {
		if nodes[i] == nodes[i-1] {
			return nil, fmt.Errorf("duplicate node %q", nodes[i])
		}
	}

	return &Jump{nodes: nodes}, nil
}

func (j *Jump) Owner(key string) NodeID {
	return j.nodes[jumpBucket(hash64(key), len(j.nodes))]
}

// r is a random fraction between 0 and 1 that is never 0
func jumpBucket(key uint64, buckets int) int {
	b, j := int64(-1), int64(0)
	for j < int64(buckets) {
		b = j
		key = key*2862933555777941757 + 1
		r := float64((key>>33)+1) / float64(1<<31)
		j = int64(float64(b+1) / r)
	}
	return int(b)
}
