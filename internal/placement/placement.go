package placement

import (
	"errors"
	"fmt"
)

type NodeID string

var (
	ErrLastNode    = errors.New("cannot remove the last node")
	ErrUnknownNode = errors.New("unknown node")
)

type Placement interface {
	Owner(key string) NodeID
	Add(n NodeID) error
	Remove(n NodeID) error
	Members() []NodeID
	Save() ([]byte, error)
}

// New builds a placement by algorithm name. vnodes is used only by ring and
// size only by Maglev.
func New(algorithm string, nodes []NodeID, vnodes, size int) (Placement, error) {
	var p Placement
	var err error
	switch algorithm {
	case "modulo":
		p, err = NewModulo(nodes)
	case "rendezvous":
		p, err = NewRendezvous(nodes)
	case "ring":
		p, err = NewRing(nodes, vnodes)
	case "jump":
		p, err = NewJump(nodes)
	case "maglev":
		p, err = NewMaglev(nodes, size)
	default:
		return nil, fmt.Errorf("unknown algorithm %q", algorithm)
	}
	// On error p holds a nil pointer, which is still a non-nil Placement.
	if err != nil {
		return nil, err
	}
	return p, nil
}
