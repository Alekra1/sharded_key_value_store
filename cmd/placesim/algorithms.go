package main

import (
	"fmt"

	"github.com/Alekra1/sharded_key_value_store/internal/placement"
)

type algorithm struct {
	name  string
	param int
	build func(members []placement.NodeID) (placement.Placement, error)
}

func algorithms() []algorithm {
	list := []algorithm{
		{name: "modulo", build: func(m []placement.NodeID) (placement.Placement, error) {
			return placement.NewModulo(m)
		}},
		{name: "rendezvous", build: func(m []placement.NodeID) (placement.Placement, error) {
			return placement.NewRendezvous(m)
		}},
	}
	for _, v := range []int{1, 10, 100, 1000} {
		list = append(list, algorithm{name: "ring", param: v, build: func(m []placement.NodeID) (placement.Placement, error) {
			return placement.NewRing(m, v)
		}})
	}
	return list
}

func nodeIDs(n int) []placement.NodeID {
	nodes := make([]placement.NodeID, n)
	for i := range n {
		nodes[i] = placement.NodeID(fmt.Sprintf("node-%d", i))
	}
	return nodes
}
