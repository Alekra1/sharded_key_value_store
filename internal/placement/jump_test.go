package placement

import "testing"

func newJump(members []NodeID) (Placement, error) { return NewJump(members) }

// Jump only promises minimal movement when the last node is removed, so the
// shared remove test uses node-9. TestJumpRemoveMiddleNode covers node-3
func TestJumpDeterministic(t *testing.T)       { testDeterministic(t, newJump) }
func TestJumpOwnerIsMember(t *testing.T)       { testOwnerIsMember(t, newJump) }
func TestJumpMemberOrderIgnored(t *testing.T)  { testMemberOrderIgnored(t, newJump) }
func TestJumpBalanced(t *testing.T)            { testBalanced(t, newJump, 0.05) }
func TestJumpMembershipRules(t *testing.T)     { testMembershipRules(t, newJump) }
func TestJumpMembersAfterChanges(t *testing.T) { testMembersAfterChanges(t, newJump) }

func TestJumpAddMovesKeysOnlyToNewNode(t *testing.T) {
	testAddMovesKeysOnlyToNewNode(t, newJump)
}

func TestJumpRemoveLastMovesOnlyItsKeys(t *testing.T) {
	testRemoveMovesOnlyRemovedNodesKeys(t, newJump, "node-9")
}

// The last node fills the hole, so only keys of the removed node and the
// last node can move
func TestJumpRemoveMiddleNode(t *testing.T) {
	const removed, last = NodeID("node-3"), NodeID("node-9")
	p := mustNew(t, newJump, nodeIDs(propertyNodes))
	before := owners(p, propertyKeys)
	if err := p.Remove(removed); err != nil {
		t.Fatal(err)
	}

	if got := p.Members()[3]; got != last {
		t.Fatalf("Members()[3] = %q after removing %q, want %q", got, removed, last)
	}

	moved := 0
	for i, owner := range owners(p, propertyKeys) {
		if owner == before[i] {
			continue
		}
		if before[i] != removed && before[i] != last {
			t.Fatalf("key-%d moved from %q to %q", i, before[i], owner)
		}
		moved++
	}
	t.Logf("moved %d of %d keys (%.1f%%)", moved, propertyKeys, 100*float64(moved)/propertyKeys)
}

func TestJumpBucketKnownValues(t *testing.T) {
	key := hash64("cat")
	cases := []struct{ buckets, want int }{
		{1, 0},
		{4, 3},
		{20, 18},
		{100, 59},
		{1000, 256},
	}
	for _, c := range cases {
		if got := jumpBucket(key, c.buckets); got != c.want {
			t.Errorf("jumpBucket(hash64(\"cat\"), %d) = %d, want %d", c.buckets, got, c.want)
		}
	}
}
