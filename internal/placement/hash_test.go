package placement

import (
	"strconv"
	"testing"
)

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

func TestHash64SpreadsTokensOfOneNode(t *testing.T) {
	const tokens = 100
	const regions = 16

	var hit [regions]bool
	for i := range tokens {
		hit[hash64("node-A", strconv.Itoa(i))>>60] = true
	}

	covered := 0
	for _, h := range hit {
		if h {
			covered++
		}
	}
	if covered < 12 {
		t.Fatalf("%d tokens of one node landed in only %d of %d ring regions; want at least 12", tokens, covered, regions)
	}
}
