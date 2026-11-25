package placement

import (
	"fmt"
	"testing"
)

// benchSink stops the compiler from optimizing away Owner calls in benchmarks.
var benchSink NodeID

func BenchmarkOwner(b *testing.B) {
	keys := make([]string, 1024)
	for i := range keys {
		keys[i] = fmt.Sprintf("key-%d", i)
	}

	algos := []struct {
		name  string
		build func([]NodeID) (Placement, error)
	}{
		{"modulo", func(m []NodeID) (Placement, error) { return NewModulo(m) }},
		{"rendezvous", func(m []NodeID) (Placement, error) { return NewRendezvous(m) }},
		{"jump", func(m []NodeID) (Placement, error) { return NewJump(m) }},
		{"ring-1", func(m []NodeID) (Placement, error) { return NewRing(m, 1) }},
		{"ring-10", func(m []NodeID) (Placement, error) { return NewRing(m, 10) }},
		{"ring-100", func(m []NodeID) (Placement, error) { return NewRing(m, 100) }},
		{"ring-1000", func(m []NodeID) (Placement, error) { return NewRing(m, 1000) }},
		{"maglev-65537", func(m []NodeID) (Placement, error) { return NewMaglev(m, 65537) }},
		{"maglev-655373", func(m []NodeID) (Placement, error) { return NewMaglev(m, 655373) }},
	}

	for _, n := range []int{3, 10, 100, 1000} {
		members := make([]NodeID, n)
		for i := range n {
			members[i] = NodeID(fmt.Sprintf("node-%d", i))
		}
		for _, a := range algos {
			p, err := a.build(members)
			if err != nil {
				b.Fatalf("%s n=%d: %v", a.name, n, err)
			}
			b.Run(fmt.Sprintf("%s/n=%d", a.name, n), func(b *testing.B) {
				var owner NodeID
				for i := range b.N {
					owner = p.Owner(keys[i%1024])
				}
				benchSink = owner
			})
		}
	}
}
