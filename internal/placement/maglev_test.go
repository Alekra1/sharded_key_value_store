package placement

import (
	"slices"
	"testing"
)

const maglevTestSize = 65537

func newMaglev(members []NodeID) (Placement, error) { return NewMaglev(members, maglevTestSize) }

// Maglev rebuilds its table, so some keys can move between nodes that are still in the cluster
// It skips the two strict disruption tests; the FewExtraKeys tests replace them for maglev
func TestMaglevDeterministic(t *testing.T)       { testDeterministic(t, newMaglev) }
func TestMaglevOwnerIsMember(t *testing.T)       { testOwnerIsMember(t, newMaglev) }
func TestMaglevMemberOrderIgnored(t *testing.T)  { testMemberOrderIgnored(t, newMaglev) }
func TestMaglevBalanced(t *testing.T)            { testBalanced(t, newMaglev, 0.05) }
func TestMaglevMembershipRules(t *testing.T)     { testMembershipRules(t, newMaglev) }
func TestMaglevMembersAfterChanges(t *testing.T) { testMembersAfterChanges(t, newMaglev) }

func TestMaglevAddMovesFewExtraKeys(t *testing.T) {
	p := mustNew(t, newMaglev, nodeIDs(propertyNodes))
	before := owners(p, propertyKeys)
	if err := p.Add("node-new"); err != nil {
		t.Fatal(err)
	}

	needed, extra := countMoves(before, owners(p, propertyKeys), "", "node-new")
	checkMoves(t, needed, extra, 1.0/(propertyNodes+1))
}

func TestMaglevRemoveMovesFewExtraKeys(t *testing.T) {
	p := mustNew(t, newMaglev, nodeIDs(propertyNodes))
	before := owners(p, propertyKeys)
	if err := p.Remove("node-3"); err != nil {
		t.Fatal(err)
	}

	needed, extra := countMoves(before, owners(p, propertyKeys), "node-3", "")
	checkMoves(t, needed, extra, 1.0/propertyNodes)
}

// A move is needed when the old owner left or the new owner is the node that joined. Any other move is extra
func countMoves(before, after []NodeID, left, joined NodeID) (needed, extra int) {
	for i := range before {
		if before[i] == after[i] {
			continue
		}
		if before[i] == left || after[i] == joined {
			needed++
		} else {
			extra++
		}
	}
	return needed, extra
}

func checkMoves(t *testing.T, needed, extra int, ideal float64) {
	t.Helper()
	neededShare := float64(needed) / propertyKeys
	extraShare := float64(extra) / propertyKeys
	t.Logf("needed %.2f%% (ideal %.2f%%), extra %.2f%%", 100*neededShare, 100*ideal, 100*extraShare)

	if neededShare < ideal-0.02 || neededShare > ideal+0.02 {
		t.Errorf("needed moves %.2f%%; want %.2f%% ± 2", 100*neededShare, 100*ideal)
	}
	if extraShare > 0.01 {
		t.Errorf("extra moves %.2f%%; want at most 1%%", 100*extraShare)
	}
}

func TestMaglevSlotsPerNodeDifferByAtMostOne(t *testing.T) {
	for _, n := range []int{1, 3, 10, 100} {
		mg, err := NewMaglev(nodeIDs(n), maglevTestSize)
		if err != nil {
			t.Fatal(err)
		}

		slots := map[NodeID]int{}
		for _, owner := range mg.table {
			slots[owner]++
		}
		fewest, most := maglevTestSize, 0
		for _, count := range slots {
			fewest = min(fewest, count)
			most = max(most, count)
		}
		if len(slots) != n || most-fewest > 1 {
			t.Errorf("%d nodes: %d own slots, %d to %d slots each; want all %d, at most 1 apart", n, len(slots), fewest, most, n)
		}
	}
}

func TestMaglevRejectsBadInput(t *testing.T) {
	for _, size := range []int{0, 1, 8, 65536} {
		if _, err := NewMaglev([]NodeID{"A"}, size); err == nil {
			t.Errorf("constructor accepted table size %d, which is not prime", size)
		}
	}
	if _, err := NewMaglev([]NodeID{"A", ""}, 7); err == nil {
		t.Error("constructor accepted an empty node name")
	}

	mg, err := NewMaglev([]NodeID{"A"}, 7)
	if err != nil {
		t.Fatal(err)
	}
	if err := mg.Add(""); err == nil {
		t.Error("Add accepted an empty node name")
	}
}

func TestMaglevKnownValues(t *testing.T) {
	mg, err := NewMaglev([]NodeID{"A", "B", "C"}, 7)
	if err != nil {
		t.Fatal(err)
	}

	want := []NodeID{"C", "B", "C", "A", "A", "B", "A"}
	if !slices.Equal(mg.table, want) {
		t.Fatalf("table = %q, want %q", mg.table, want)
	}

	cases := []struct {
		key  string
		want NodeID
	}{
		{"cat", "B"},
		{"bee", "A"},
		{"yak", "C"},
		{"elk", "B"},
		{"owl", "A"},
	}
	for _, c := range cases {
		if got := mg.Owner(c.key); got != c.want {
			t.Errorf("Owner(%q) = %q, want %q", c.key, got, c.want)
		}
	}
}

func TestMaglevAddKeepsTurnOrderSorted(t *testing.T) {
	mg, err := NewMaglev(nodeIDs(propertyNodes), maglevTestSize)
	if err != nil {
		t.Fatal(err)
	}
	if err := mg.Add("node-25"); err != nil {
		t.Fatal(err)
	}

	fresh, err := NewMaglev(append(nodeIDs(propertyNodes), "node-25"), maglevTestSize)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(mg.table, fresh.table) {
		t.Fatal("table after Add differs from a fresh build with the same nodes")
	}
}
