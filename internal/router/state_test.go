package router

import (
	"errors"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/Alekra1/sharded_key_value_store/internal/placement"
)

var testURLs = map[placement.NodeID]string{
	"A": "http://localhost:8001",
	"B": "http://localhost:8002",
	"C": "http://localhost:8003",
}

func TestStateRoundTrip(t *testing.T) {
	p, err := placement.NewJump([]placement.NodeID{"A", "B", "C", "D"})
	if err != nil {
		t.Fatal(err)
	}
	// After this removal jump's order is A D C, which a plain rebuild would lose.
	if err := p.Remove("B"); err != nil {
		t.Fatal(err)
	}
	urls := map[placement.NodeID]string{"A": "http://a", "C": "http://c", "D": "http://d"}
	path := filepath.Join(t.TempDir(), "state.json")
	if err := saveState(path, p, urls); err != nil {
		t.Fatal(err)
	}

	loaded, loadedURLs, err := LoadState(path)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(loaded.Members(), p.Members()) {
		t.Errorf("members %v, want %v", loaded.Members(), p.Members())
	}
	for _, key := range []string{"cat", "dog", "fox", "user 1+2"} {
		if loaded.Owner(key) != p.Owner(key) {
			t.Errorf("owner of %q changed after load", key)
		}
	}
	if !maps.Equal(loadedURLs, urls) {
		t.Errorf("urls %v, want %v", loadedURLs, urls)
	}
}

func TestSaveStateReplacesFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state.json")
	first, err := placement.NewRendezvous([]placement.NodeID{"A", "B"})
	if err != nil {
		t.Fatal(err)
	}
	second, err := placement.NewRendezvous([]placement.NodeID{"A", "B", "C"})
	if err != nil {
		t.Fatal(err)
	}
	if err := saveState(path, first, testURLs); err != nil {
		t.Fatal(err)
	}
	if err := saveState(path, second, testURLs); err != nil {
		t.Fatal(err)
	}

	loaded, _, err := LoadState(path)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(loaded.Members(), second.Members()) {
		t.Errorf("members %v, want the second save %v", loaded.Members(), second.Members())
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Errorf("directory has %d files, want only state.json (temp files left behind?)", len(entries))
	}
}

func TestLoadStateErrors(t *testing.T) {
	dir := t.TempDir()
	if _, _, err := LoadState(filepath.Join(dir, "missing.json")); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("missing file: got %v, want fs.ErrNotExist", err)
	}

	cases := map[string]string{
		"broken JSON":       `{"placement":`,
		"unknown algorithm": `{"placement":{"algorithm":"chord","nodes":["A"]},"urls":{}}`,
		"no placement":      `{"urls":{"A":"http://a"}}`,
	}
	for name, content := range cases {
		path := filepath.Join(dir, "state.json")
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
		if p, _, err := LoadState(path); err == nil || p != nil {
			t.Errorf("%s: got %v, %v; want nil and an error", name, p, err)
		}
	}
}
