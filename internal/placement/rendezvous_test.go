package placement

import "testing"

func newRendezvous(members []NodeID) (Placement, error) { return NewRendezvous(members) }

func TestRendezvousDeterministic(t *testing.T)      { testDeterministic(t, newRendezvous) }
func TestRendezvousOwnerIsMember(t *testing.T)      { testOwnerIsMember(t, newRendezvous) }
func TestRendezvousMemberOrderIgnored(t *testing.T) { testMemberOrderIgnored(t, newRendezvous) }
func TestRendezvousBalanced(t *testing.T)           { testBalanced(t, newRendezvous, 0.05) }
func TestRendezvousMembershipRules(t *testing.T)    { testMembershipRules(t, newRendezvous) }

func TestRendezvousAddMovesKeysOnlyToNewNode(t *testing.T) {
	testAddMovesKeysOnlyToNewNode(t, newRendezvous)
}

func TestRendezvousRemoveMovesOnlyRemovedNodesKeys(t *testing.T) {
	testRemoveMovesOnlyRemovedNodesKeys(t, newRendezvous)
}
