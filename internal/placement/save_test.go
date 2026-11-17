package placement

import (
	"slices"
	"testing"
)

func TestSaveLoadKeepsOwners(t *testing.T) {
	cases := []struct {
		name string
		newP newPlacementFunc
	}{
		{"modulo", newModulo},
		{"rendezvous", newRendezvous},
		{"ring", newRing},
		{"jump", newJump},
		{"maglev", newMaglev},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			p := mustNew(t, c.newP, nodeIDs(propertyNodes))
			// Removing a middle node leaves jump's list out of sorted order.
			if err := p.Remove("node-3"); err != nil {
				t.Fatal(err)
			}
			if err := p.Add("node-10"); err != nil {
				t.Fatal(err)
			}

			data, err := p.Save()
			if err != nil {
				t.Fatal(err)
			}
			loaded, err := Load(data)
			if err != nil {
				t.Fatalf("Load(%s): %v", data, err)
			}
			if !slices.Equal(loaded.Members(), p.Members()) {
				t.Fatalf("members %v after load, want %v", loaded.Members(), p.Members())
			}
			if !slices.Equal(owners(loaded, propertyKeys), owners(p, propertyKeys)) {
				t.Fatal("loaded placement gives different owners")
			}
		})
	}
}

func TestSaveFormat(t *testing.T) {
	j, err := NewJump([]NodeID{"a", "b", "c"})
	if err != nil {
		t.Fatal(err)
	}
	if err := j.Remove("a"); err != nil {
		t.Fatal(err)
	}
	r, err := NewRing([]NodeID{"b", "a"}, 100)
	if err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		p    Placement
		want string
	}{
		{j, `{"algorithm":"jump","nodes":["c","b"]}`},
		{r, `{"algorithm":"ring","vnodes":100,"nodes":["a","b"]}`},
	}
	for _, c := range cases {
		data, err := c.p.Save()
		if err != nil {
			t.Fatal(err)
		}
		if string(data) != c.want {
			t.Errorf("Save() = %s, want %s", data, c.want)
		}
	}
}

func TestLoadRejects(t *testing.T) {
	cases := []string{
		`not json`,
		`{"algorithm":"chord","nodes":["a"]}`,
		`{"algorithm":"modulo","nodes":[]}`,
		`{"algorithm":"ring","nodes":["a"]}`,
		`{"algorithm":"maglev","size":100,"nodes":["a"]}`,
		`{"algorithm":"jump","nodes":["a","a"]}`,
	}
	for _, data := range cases {
		p, err := Load([]byte(data))
		if err == nil || p != nil {
			t.Errorf("Load(%s) = %v, %v; want nil and an error", data, p, err)
		}
	}
}
