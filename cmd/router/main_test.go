package main

import (
	"maps"
	"testing"

	"github.com/Alekra1/sharded_key_value_store/internal/placement"
)

func TestParseNodes(t *testing.T) {
	got, err := parseNodes("A=http://localhost:8001, B=http://localhost:8002/")
	if err != nil {
		t.Fatal(err)
	}
	want := map[placement.NodeID]string{"A": "http://localhost:8001", "B": "http://localhost:8002"}
	if !maps.Equal(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}

	for _, bad := range []string{
		"",
		"A",
		"=http://localhost:8001",
		"A=localhost:8001",
		"A=http://localhost:8001,A=http://localhost:8002",
	} {
		if _, err := parseNodes(bad); err == nil {
			t.Errorf("parseNodes(%q) accepted bad input", bad)
		}
	}
}
