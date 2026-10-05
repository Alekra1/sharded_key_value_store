package placement

import (
	"fmt"
	"testing"
)

func newModulo(members []NodeID) (Placement, error) { return NewModulo(members) }

// Modulo is not consistent, so it skips the two minimal-disruption tests
// Instead, TestModuloDisruption shows how many keys it moves
func TestModuloDeterministic(t *testing.T)      { testDeterministic(t, newModulo) }
func TestModuloOwnerIsMember(t *testing.T)      { testOwnerIsMember(t, newModulo) }
func TestModuloMemberOrderIgnored(t *testing.T) { testMemberOrderIgnored(t, newModulo) }
func TestModuloBalanced(t *testing.T)           { testBalanced(t, newModulo, 0.05) }
func TestModuloMembershipRules(t *testing.T)    { testMembershipRules(t, newModulo) }

func TestModuloDisruption(t *testing.T) {
	m, err := NewModulo([]NodeID{"A", "B", "C"})
	if err != nil {
		t.Fatal(err)
	}

	const keyCount = 10_000
	before := make([]NodeID, keyCount)
	for i := range keyCount {
		before[i] = m.Owner(fmt.Sprintf("key-%d", i))
	}
	if err := m.Add("D"); err != nil {
		t.Fatal(err)
	}

	moved := 0
	for i := range keyCount {
		if m.Owner(fmt.Sprintf("key-%d", i)) != before[i] {
			moved++
		}
	}
	t.Logf("moved %d of %d keys (%.1f%%)", moved, keyCount, 100*float64(moved)/keyCount)
	if moved <= 5_000 {
		t.Fatalf("only %d of %d keys moved; want more than 5,000", moved, keyCount)
	}
}
