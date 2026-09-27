package placement

import "testing"

func TestHash64DistinguishesPartBoundaries(t *testing.T) {
	if hash64("ab", "c") == hash64("a", "bc") {
		t.Fatal("different part boundaries produced the same hash")
	}
}

func TestHash64Deterministic(t *testing.T) {
	first := hash64("key", "node")
	if second := hash64("key", "node"); first != second {
		t.Fatalf("same input produced different hashes: %d and %d", first, second)
	}
}
