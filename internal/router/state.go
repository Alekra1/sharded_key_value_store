package router

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"slices"

	"github.com/Alekra1/sharded_key_value_store/internal/placement"
)

// state is the router's file. Placement holds the bytes from Save as they
// are, so the file shows them as JSON instead of an encoded string.
type state struct {
	Placement json.RawMessage             `json:"placement"`
	URLs      map[placement.NodeID]string `json:"urls"`
}

// InitState creates the state file of a new cluster. It refuses to replace an
// existing file, which would lose the placement of a cluster that has data.
func InitState(path, algorithm string, urls map[placement.NodeID]string, vnodes, size int) error {
	if _, err := os.Stat(path); err == nil {
		return fmt.Errorf("%s: %w", path, fs.ErrExist)
	}
	// The constructors sort the nodes, so the map's random order doesn't matter.
	p, err := placement.New(algorithm, slices.Collect(maps.Keys(urls)), vnodes, size)
	if err != nil {
		return err
	}
	return saveState(path, p, urls)
}

func saveState(path string, p placement.Placement, urls map[placement.NodeID]string) error {
	saved, err := p.Save()
	if err != nil {
		return err
	}
	data, err := json.MarshalIndent(state{Placement: saved, URLs: urls}, "", "  ")
	if err != nil {
		return err
	}
	return replaceFile(path, data)
}

func loadState(path string) (state, placement.Placement, error) {
	var s state
	data, err := os.ReadFile(path)
	if err != nil {
		return s, nil, err
	}
	if err := json.Unmarshal(data, &s); err != nil {
		return s, nil, fmt.Errorf("%s: %w", path, err)
	}
	p, err := placement.Load(s.Placement)
	if err != nil {
		return s, nil, fmt.Errorf("%s: %w", path, err)
	}
	return s, p, nil
}

func replaceFile(path string, data []byte) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), filepath.Base(path)+".tmp-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())

	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}
