package main

import (
	"fmt"
	"math/rand/v2"
)

func generateKeys(seed uint64, count int) []string {
	r := rand.New(rand.NewPCG(seed, 0))
	keys := make([]string, count)
	for i := range count {
		keys[i] = fmt.Sprintf("key-%016x", r.Uint64())
	}
	return keys
}
