package main

import (
	"runtime"

	"github.com/Alekra1/sharded_key_value_store/internal/placement"
)

func measureSavedBytes(p placement.Placement) (int, error) {
	saved, err := p.Save()
	if err != nil {
		return 0, err
	}
	return len(saved), nil
}

// buildAndMeasureMem builds a placement and estimates the live heap bytes it
// retains. Running GC after build drops temporary slices from construction.
func buildAndMeasureMem(build func() (placement.Placement, error)) (p placement.Placement, heapBytes uint64, err error) {
	runtime.GC()
	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)

	p, err = build()
	if err != nil {
		return nil, 0, err
	}

	runtime.GC()
	runtime.ReadMemStats(&after)
	runtime.KeepAlive(p)

	if after.HeapAlloc <= before.HeapAlloc {
		return p, 0, nil
	}
	return p, after.HeapAlloc - before.HeapAlloc, nil
}
