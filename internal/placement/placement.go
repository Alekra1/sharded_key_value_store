package placement

import "errors"

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
