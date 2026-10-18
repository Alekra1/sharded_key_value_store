package main

import (
	"encoding/csv"
	"fmt"
	"io"
	"strconv"

	"github.com/Alekra1/sharded_key_value_store/internal/placement"
)

var header = []string{
	"seed", "keys", "algorithm", "param", "nodes",
	"max_over_mean", "cv",
	"add_moved", "add_ideal",
	"remove_last_moved", "remove_middle_moved", "remove_ideal",
}

func run(w io.Writer, seeds []uint64, keyCount int, nodeCounts []int) error {
	out := csv.NewWriter(w)
	if err := out.Write(header); err != nil {
		return err
	}
	for _, seed := range seeds {
		keys := generateKeys(seed, keyCount)
		for _, n := range nodeCounts {
			members := nodeIDs(seed, n)
			for _, a := range algorithms() {
				measured, err := measureRow(a, members, nodeID(seed, n), keys)
				if err != nil {
					return fmt.Errorf("%s %d with %d nodes: %w", a.name, a.param, n, err)
				}
				row := append([]string{
					strconv.FormatUint(seed, 10),
					strconv.Itoa(keyCount),
					a.name,
					strconv.Itoa(a.param),
					strconv.Itoa(n),
				}, measured...)
				if err := out.Write(row); err != nil {
					return err
				}
			}
		}
	}
	out.Flush()
	return out.Error()
}

func measureRow(a algorithm, members []placement.NodeID, newNode placement.NodeID, keys []string) ([]string, error) {
	n := len(members)

	p, err := a.build(members)
	if err != nil {
		return nil, err
	}
	balance := measureBalance(p, keys)

	p, err = a.build(members)
	if err != nil {
		return nil, err
	}
	added, err := measureAdd(p, keys, newNode)
	if err != nil {
		return nil, err
	}

	p, err = a.build(members)
	if err != nil {
		return nil, err
	}
	removedLast, err := measureRemove(p, keys, members[n-1])
	if err != nil {
		return nil, err
	}

	p, err = a.build(members)
	if err != nil {
		return nil, err
	}
	removedMiddle, err := measureRemove(p, keys, members[n/2])
	if err != nil {
		return nil, err
	}

	return []string{
		formatFloat(balance.MaxOverMean),
		formatFloat(balance.CV),
		formatFloat(added),
		formatFloat(1 / float64(n+1)),
		formatFloat(removedLast),
		formatFloat(removedMiddle),
		formatFloat(1 / float64(n)),
	}, nil
}

func formatFloat(x float64) string {
	return strconv.FormatFloat(x, 'f', 6, 64)
}
