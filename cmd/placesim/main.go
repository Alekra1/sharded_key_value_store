package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"
)

func main() {
	keyCount := flag.Int("keys", 1_000_000, "number of keys per seed")
	seedCount := flag.Int("seeds", 3, "number of seeds, run as 1, 2, ...")
	nodes := flag.String("nodes", "3,10,100,1000", "comma-separated node counts")
	outPath := flag.String("out", "", "CSV output file (default: print to the terminal)")
	flag.Parse()

	if err := simulate(*keyCount, *seedCount, *nodes, *outPath); err != nil {
		fmt.Fprintln(os.Stderr, "placesim:", err)
		os.Exit(1)
	}
}

func simulate(keyCount, seedCount int, nodes, outPath string) error {
	if keyCount < 1 || seedCount < 1 {
		return errors.New("keys and seeds must be at least 1")
	}
	nodeCounts, err := parseNodes(nodes)
	if err != nil {
		return err
	}
	seeds := make([]uint64, seedCount)
	for i := range seedCount {
		seeds[i] = uint64(i + 1)
	}

	if outPath == "" {
		return run(os.Stdout, seeds, keyCount, nodeCounts)
	}
	f, err := os.Create(outPath)
	if err != nil {
		return err
	}
	if err := run(f, seeds, keyCount, nodeCounts); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}

func parseNodes(s string) ([]int, error) {
	parts := strings.Split(s, ",")
	counts := make([]int, len(parts))
	for i, part := range parts {
		n, err := strconv.Atoi(strings.TrimSpace(part))
		if err != nil {
			return nil, fmt.Errorf("bad node count %q", part)
		}
		if n < 2 {
			return nil, fmt.Errorf("node count %d: need at least 2", n)
		}
		counts[i] = n
	}
	return counts, nil
}
