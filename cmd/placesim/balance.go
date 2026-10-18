package main

import (
	"math"

	"github.com/Alekra1/sharded_key_value_store/internal/placement"
)

type balanceResult struct {
	MaxOverMean float64
	CV          float64
}

func measureBalance(p placement.Placement, keys []string) balanceResult {
	members := p.Members()
	counts := make(map[placement.NodeID]int, len(members))
	for _, n := range members {
		counts[n] = 0
	}
	for _, k := range keys {
		counts[p.Owner(k)]++
	}

	mean := float64(len(keys)) / float64(len(members))
	maxCount := 0
	sumSquares := 0.0
	for _, c := range counts {
		maxCount = max(maxCount, c)
		d := float64(c) - mean
		sumSquares += d * d
	}
	stddev := math.Sqrt(sumSquares / float64(len(members)))

	return balanceResult{
		MaxOverMean: float64(maxCount) / mean,
		CV:          stddev / mean,
	}
}
