package placement

import (
	"errors"
	"fmt"
	"math"
	"slices"
	"testing"
)

// Shared property tests

type newPlacementFunc func(members []NodeID) (Placement, error)

const (
	propertyNodes = 10
	propertyKeys  = 10_000
)

func nodeIDs(n int) []NodeID {
	nodes := make([]NodeID, n)
	for i := range n {
		nodes[i] = NodeID(fmt.Sprintf("node-%d", i))
	}
	return nodes
}

func mustNew(t *testing.T, newP newPlacementFunc, members []NodeID) Placement {
	t.Helper()
	p, err := newP(members)
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func owners(p Placement, n int) []NodeID {
	result := make([]NodeID, n)
	for i := range n {
		result[i] = p.Owner(fmt.Sprintf("key-%d", i))
	}
	return result
}

func testDeterministic(t *testing.T, newP newPlacementFunc) {
	first := owners(mustNew(t, newP, nodeIDs(propertyNodes)), propertyKeys)
	second := owners(mustNew(t, newP, nodeIDs(propertyNodes)), propertyKeys)
	if !slices.Equal(first, second) {
		t.Fatal("two placements built from the same members disagree on owners")
	}
}

func testOwnerIsMember(t *testing.T, newP newPlacementFunc) {
	p := mustNew(t, newP, nodeIDs(propertyNodes))
	members := p.Members()
	for i, owner := range owners(p, propertyKeys) {
		if !slices.Contains(members, owner) {
			t.Fatalf("Owner(\"key-%d\") = %q, not a member", i, owner)
		}
	}
}

func testMemberOrderIgnored(t *testing.T, newP newPlacementFunc) {
	reversed := nodeIDs(propertyNodes)
	slices.Reverse(reversed)

	sorted := owners(mustNew(t, newP, nodeIDs(propertyNodes)), propertyKeys)
	if !slices.Equal(sorted, owners(mustNew(t, newP, reversed), propertyKeys)) {
		t.Fatal("reversing the member list changed owners")
	}
}

// testBalanced checks that every node's key count is within maxDeviation
// (0.05 = 5%) of the mean.
func testBalanced(t *testing.T, newP newPlacementFunc, maxDeviation float64) {
	const keyCount = 100_000
	p := mustNew(t, newP, nodeIDs(propertyNodes))

	counts := make(map[NodeID]int)
	for _, owner := range owners(p, keyCount) {
		counts[owner]++
	}

	mean := float64(keyCount) / propertyNodes
	for _, n := range p.Members() {
		if deviation := math.Abs(float64(counts[n])-mean) / mean; deviation > maxDeviation {
			t.Errorf("node %q got %d keys, %.1f%% away from the mean %.0f; want within %.1f%%",
				n, counts[n], 100*deviation, mean, 100*maxDeviation)
		}
	}
}

func testAddMovesKeysOnlyToNewNode(t *testing.T, newP newPlacementFunc) {
	p := mustNew(t, newP, nodeIDs(propertyNodes))
	before := owners(p, propertyKeys)
	if err := p.Add("node-new"); err != nil {
		t.Fatal(err)
	}

	moved := 0
	for i, owner := range owners(p, propertyKeys) {
		if owner == before[i] {
			continue
		}
		if owner != "node-new" {
			t.Fatalf("key-%d moved from %q to %q, not to the new node", i, before[i], owner)
		}
		moved++
	}

	got := float64(moved) / propertyKeys
	want := 1.0 / (propertyNodes + 1)
	if math.Abs(got-want) > 0.02 {
		t.Fatalf("%.1f%% of keys moved; want %.1f%% ± 2", 100*got, 100*want)
	}
}

// Only removed node's keys move, and no other
func testRemoveMovesOnlyRemovedNodesKeys(t *testing.T, newP newPlacementFunc) {
	const removed = NodeID("node-3")
	p := mustNew(t, newP, nodeIDs(propertyNodes))
	before := owners(p, propertyKeys)
	if err := p.Remove(removed); err != nil {
		t.Fatal(err)
	}

	for i, owner := range owners(p, propertyKeys) {
		if owner == removed {
			t.Fatalf("key-%d is still owned by the removed node", i)
		}
		if before[i] != removed && owner != before[i] {
			t.Fatalf("key-%d moved from %q to %q, but %q was not removed", i, before[i], owner, before[i])
		}
	}
}

func testMembershipRules(t *testing.T, newP newPlacementFunc) {
	if _, err := newP(nil); err == nil {
		t.Fatal("constructor accepted no members")
	}
	if _, err := newP([]NodeID{"A", "A"}); err == nil {
		t.Fatal("constructor accepted duplicate members")
	}

	p := mustNew(t, newP, []NodeID{"A"})
	if err := p.Add("A"); err == nil {
		t.Fatal("Add accepted a duplicate member")
	}
	if err := p.Remove("Z"); !errors.Is(err, ErrUnknownNode) {
		t.Fatalf("Remove(\"Z\") = %v, want ErrUnknownNode", err)
	}
	if err := p.Remove("A"); !errors.Is(err, ErrLastNode) {
		t.Fatalf("Remove(\"A\") = %v, want ErrLastNode", err)
	}

	members := p.Members()
	if len(members) != 1 || members[0] != "A" {
		t.Fatalf("Members() = %v after rejected changes, want [A]", members)
	}
	members[0] = "X"
	if got := p.Members()[0]; got != "A" {
		t.Fatalf("changing the Members() result changed the placement: got %q", got)
	}
}
