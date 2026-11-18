package router

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/Alekra1/sharded_key_value_store/internal/placement"
)

// state is the router's file. Placement holds the bytes from Save as they
// are, so the file shows them as JSON instead of an encoded string.
type state struct {
	Placement json.RawMessage             `json:"placement"`
	URLs      map[placement.NodeID]string `json:"urls"`
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

func LoadState(path string) (placement.Placement, map[placement.NodeID]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, nil, err
	}
	var s state
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, nil, fmt.Errorf("%s: %w", path, err)
	}
	p, err := placement.Load(s.Placement)
	if err != nil {
		return nil, nil, fmt.Errorf("%s: %w", path, err)
	}
	return p, s.URLs, nil
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
