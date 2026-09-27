package placement

import (
	"errors"
	"fmt"
	"slices"
	"testing"
)

func TestModuloDeterminism(t *testing.T) {
	m, err := NewModulo([]NodeID{"C", "A", "B"})
	if err != nil {
		t.Fatal(err)
	}

	for i := range 100 {
		key := fmt.Sprintf("key-%d", i)
		if first, second := m.Owner(key), m.Owner(key); first != second {
			t.Fatalf("Owner(%q) changed from %q to %q", key, first, second)
		}
	}
}

func TestModuloMembership(t *testing.T) {
	m, err := NewModulo([]NodeID{"C", "A", "B"})
	if err != nil {
		t.Fatal(err)
	}

	members := m.Members()
	if len(members) != 3 {
		t.Fatalf("Members() returned %d nodes, want 3", len(members))
	}
	for i, want := range []NodeID{"A", "B", "C"} {
		if members[i] != want {
			t.Fatalf("Members()[%d] = %q, want %q", i, members[i], want)
		}
	}
	for i := range 100 {
		key := fmt.Sprintf("key-%d", i)
		if owner := m.Owner(key); !slices.Contains(members, owner) {
			t.Fatalf("Owner(%q) = %q, not a member", key, owner)
		}
	}

	members[0] = "X"
	if got := m.Members()[0]; got != "A" {
		t.Fatalf("changing Members() result changed internal nodes: got %q", got)
	}
	if _, err := NewModulo(nil); err == nil {
		t.Fatal("NewModulo accepted no members")
	}
	if _, err := NewModulo([]NodeID{"A", "A"}); err == nil {
		t.Fatal("NewModulo accepted duplicate members")
	}
	if err := m.Add("B"); err == nil {
		t.Fatal("Add accepted a duplicate member")
	}
}

func TestModuloLastNode(t *testing.T) {
	m, err := NewModulo([]NodeID{"A"})
	if err != nil {
		t.Fatal(err)
	}

	if err := m.Remove("Z"); !errors.Is(err, ErrUnknownNode) {
		t.Fatalf("Remove(\"Z\") = %v, want ErrUnknownNode", err)
	}
	if err := m.Remove("A"); !errors.Is(err, ErrLastNode) {
		t.Fatalf("Remove(\"A\") = %v, want ErrLastNode", err)
	}
	if members := m.Members(); len(members) != 1 || members[0] != "A" {
		t.Fatalf("last member changed after rejected removal: %v", members)
	}
}

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
