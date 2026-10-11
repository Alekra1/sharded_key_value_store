package placement

import (
	"math"
	"testing"
)

const ringTestVnodes = 1000

func newRing(members []NodeID) (Placement, error) { return NewRing(members, ringTestVnodes) }

// The ring's arcs are uneven even with 1,000 virtual nodes per node, so its
// balance is set to 10% while rendezvous and modulo use 5%
func TestRingDeterministic(t *testing.T)      { testDeterministic(t, newRing) }
func TestRingOwnerIsMember(t *testing.T)      { testOwnerIsMember(t, newRing) }
func TestRingMemberOrderIgnored(t *testing.T) { testMemberOrderIgnored(t, newRing) }
func TestRingBalanced(t *testing.T)           { testBalanced(t, newRing, 0.10) }
func TestRingMembershipRules(t *testing.T)    { testMembershipRules(t, newRing) }

func TestRingAddMovesKeysOnlyToNewNode(t *testing.T) {
	testAddMovesKeysOnlyToNewNode(t, newRing)
}

func TestRingRemoveMovesOnlyRemovedNodesKeys(t *testing.T) {
	testRemoveMovesOnlyRemovedNodesKeys(t, newRing)
}

func TestRingRejectsZeroVnodes(t *testing.T) {
	if _, err := NewRing([]NodeID{"A"}, 0); err == nil {
		t.Fatal("constructor accepted zero virtual nodes")
	}
}

func TestRingOwnerAt(t *testing.T) {
	r := &Ring{tokens: []token{{100, "A"}, {200, "B"}, {300, "C"}}}

	cases := []struct {
		position uint64
		want     NodeID
	}{
		{50, "A"},
		{100, "A"},
		{101, "B"},
		{200, "B"},
		{250, "C"},
		{300, "C"},
		{301, "A"},
		{math.MaxUint64, "A"},
	}
	for _, c := range cases {
		if got := r.ownerAt(c.position); got != c.want {
			t.Errorf("ownerAt(%d) = %q, want %q", c.position, got, c.want)
		}
	}
}
