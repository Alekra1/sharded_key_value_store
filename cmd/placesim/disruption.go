package main

import (
	"time"

	"github.com/Alekra1/sharded_key_value_store/internal/placement"
)

func ownersOf(p placement.Placement, keys []string) []placement.NodeID {
	owners := make([]placement.NodeID, len(keys))
	for i, k := range keys {
		owners[i] = p.Owner(k)
	}
	return owners
}

func fractionChanged(before, after []placement.NodeID) float64 {
	changed := 0
	for i := range before {
		if before[i] != after[i] {
			changed++
		}
	}
	return float64(changed) / float64(len(before))
}

func measureAdd(p placement.Placement, keys []string, newNode placement.NodeID) (float64, float64, error) {
	before := ownersOf(p, keys)
	start := time.Now()
	if err := p.Add(newNode); err != nil {
		return 0, 0, err
	}
	rebuildMs := float64(time.Since(start).Nanoseconds()) / 1e6
	return fractionChanged(before, ownersOf(p, keys)), rebuildMs, nil
}

func measureRemove(p placement.Placement, keys []string, node placement.NodeID) (float64, float64, error) {
	before := ownersOf(p, keys)
	start := time.Now()
	if err := p.Remove(node); err != nil {
		return 0, 0, err
	}
	rebuildMs := float64(time.Since(start).Nanoseconds()) / 1e6
	return fractionChanged(before, ownersOf(p, keys)), rebuildMs, nil
}
