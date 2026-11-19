package store

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"maps"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func newStore(t *testing.T) (*Store, string) {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "node")
	s, err := Create(dir)
	if err != nil {
		t.Fatal(err)
	}
	return s, dir
}

func reopen(t *testing.T, dir string) (*Store, int) {
	t.Helper()
	s, cut, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	return s, cut
}

func mustPut(t *testing.T, s *Store, key, value string) {
	t.Helper()
	if err := s.Put(key, value); err != nil {
		t.Fatalf("Put(%q): %v", key, err)
	}
}

func sideFiles(t *testing.T, dir string) []string {
	t.Helper()
	files, err := filepath.Glob(filepath.Join(dir, logName+".cut-*"))
	if err != nil {
		t.Fatal(err)
	}
	return files
}

func TestPutGetDelete(t *testing.T) {
	s, _ := newStore(t)
	defer s.Close()

	mustPut(t, s, "cat", "meow")
	mustPut(t, s, "cat", "purr")
	mustPut(t, s, "empty", "")
	if v, ok := s.Get("cat"); !ok || v != "purr" {
		t.Fatalf("Get(cat) = %q, %v; want purr, true", v, ok)
	}
	if v, ok := s.Get("empty"); !ok || v != "" {
		t.Fatalf("Get(empty) = %q, %v; want \"\", true", v, ok)
	}
	if _, ok := s.Get("missing"); ok {
		t.Fatal("Get(missing) found a value")
	}

	if err := s.Delete("cat"); err != nil {
		t.Fatal(err)
	}
	if _, ok := s.Get("cat"); ok {
		t.Fatal("cat still there after Delete")
	}
	if err := s.Delete("missing"); err != nil {
		t.Fatalf("Delete(missing): %v", err)
	}
}

func TestRejectsBadInput(t *testing.T) {
	s, _ := newStore(t)
	defer s.Close()

	longKey := strings.Repeat("k", maxKeyLen+1)
	longValue := strings.Repeat("v", maxValueLen+1)
	cases := []struct {
		err  error
		want error
	}{
		{s.Put("", "x"), ErrEmptyKey},
		{s.Put(longKey, "x"), ErrKeyTooLong},
		{s.Put("k", longValue), ErrValueTooLong},
		{s.Delete(""), ErrEmptyKey},
		{s.Delete(longKey), ErrKeyTooLong},
	}
	for i, c := range cases {
		if c.err != c.want {
			t.Errorf("case %d: got %v, want %v", i, c.err, c.want)
		}
	}

	mustPut(t, s, strings.Repeat("k", maxKeyLen), strings.Repeat("v", maxValueLen))
}

func TestCreateRefusesExistingLog(t *testing.T) {
	s, dir := newStore(t)
	mustPut(t, s, "cat", "meow")
	s.Close()

	if _, err := Create(dir); err == nil {
		t.Fatal("Create succeeded over an existing log")
	}
	s, _ = reopen(t, dir)
	defer s.Close()
	if v, _ := s.Get("cat"); v != "meow" {
		t.Fatalf("existing log changed: Get(cat) = %q", v)
	}
}

func TestReopenKeepsData(t *testing.T) {
	s, dir := newStore(t)
	mustPut(t, s, "cat", "meow")
	mustPut(t, s, "dog", "woof")
	mustPut(t, s, "cat", "purr")
	if err := s.Delete("dog"); err != nil {
		t.Fatal(err)
	}
	want := maps.Clone(s.data)
	s.Close()

	s, cut := reopen(t, dir)
	if cut != 0 || !maps.Equal(s.data, want) {
		t.Fatalf("after reopen: %v, cut %d; want %v, cut 0", s.data, cut, want)
	}
	mustPut(t, s, "fox", "ring")
	s.Close()

	s, _ = reopen(t, dir)
	defer s.Close()
	if v, _ := s.Get("fox"); v != "ring" {
		t.Fatal("put after reopen was lost")
	}
}

func TestOpenCutsCrashedWrite(t *testing.T) {
	s, dir := newStore(t)
	mustPut(t, s, "a", "1")
	mustPut(t, s, "b", "2")
	want := maps.Clone(s.data)
	s.Close()
	good, err := os.ReadFile(filepath.Join(dir, logName))
	if err != nil {
		t.Fatal(err)
	}
	last := encodeEntry(entry{op: opPut, key: "c", value: "3"})

	for n := 1; n < len(last); n++ {
		crashDir := filepath.Join(t.TempDir(), "node")
		if err := os.MkdirAll(crashDir, 0o755); err != nil {
			t.Fatal(err)
		}
		partial := last[:n]
		file := append(bytes.Clone(good), partial...)
		if err := os.WriteFile(filepath.Join(crashDir, logName), file, 0o644); err != nil {
			t.Fatal(err)
		}

		s, cut := reopen(t, crashDir)
		if cut != n || !maps.Equal(s.data, want) {
			t.Fatalf("%d bytes of the last write: data %v, cut %d; want %v, cut %d", n, s.data, cut, want, n)
		}
		files := sideFiles(t, crashDir)
		if len(files) != 1 {
			t.Fatalf("%d bytes of the last write: %d side files, want 1", n, len(files))
		}
		saved, err := os.ReadFile(files[0])
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(saved, partial) {
			t.Fatalf("%d bytes of the last write: side file holds %x, want %x", n, saved, partial)
		}

		mustPut(t, s, "d", "4")
		s.Close()
		s, _ = reopen(t, crashDir)
		if v, _ := s.Get("d"); v != "4" {
			t.Fatalf("%d bytes of the last write: put after the cut was lost", n)
		}
		s.Close()
	}
}

func TestOpenStopsAtDamage(t *testing.T) {
	s, dir := newStore(t)
	for _, k := range []string{"a", "b", "c", "d", "e"} {
		mustPut(t, s, k, "1")
	}
	s.Close()

	path := filepath.Join(dir, logName)
	file, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	entrySize := len(encodeEntry(entry{op: opPut, key: "a", value: "1"}))
	valueOffset := 1 + 4 + 1 + 4
	damagedAt := len(encodeHeader()) + entrySize + valueOffset
	file[damagedAt] ^= 0x01
	if err := os.WriteFile(path, file, 0o644); err != nil {
		t.Fatal(err)
	}

	s, cut := reopen(t, dir)
	defer s.Close()
	if want := map[string]string{"a": "1"}; !maps.Equal(s.data, want) {
		t.Fatalf("data %v, want %v", s.data, want)
	}
	if cut != 4*entrySize {
		t.Fatalf("cut %d, want %d", cut, 4*entrySize)
	}

	files := sideFiles(t, dir)
	if len(files) != 1 {
		t.Fatalf("%d side files, want 1", len(files))
	}
	saved, err := os.ReadFile(files[0])
	if err != nil {
		t.Fatal(err)
	}
	r := bytes.NewReader(saved[entrySize:])
	var recovered []string
	for {
		e, err := readEntry(r)
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		recovered = append(recovered, e.key)
	}
	if fmt.Sprint(recovered) != "[c d e]" {
		t.Fatalf("recovered %v from the side file, want [c d e]", recovered)
	}
}

func TestOpenRefuses(t *testing.T) {
	if _, _, err := Open(filepath.Join(t.TempDir(), "missing")); err == nil {
		t.Error("Open of a folder without a log succeeded")
	}

	dir := t.TempDir()
	for _, header := range []string{"JUNK\x01", "KVLG\x02", "KV"} {
		if err := os.WriteFile(filepath.Join(dir, logName), []byte(header), 0o644); err != nil {
			t.Fatal(err)
		}
		if _, _, err := Open(dir); err == nil {
			t.Errorf("Open accepted header %q", header)
		}
	}
}

func TestBrokenStoreRefusesWrites(t *testing.T) {
	s, _ := newStore(t)
	mustPut(t, s, "cat", "meow")
	s.log.Close()

	first := s.Put("dog", "woof")
	if first == nil {
		t.Fatal("Put on a closed log succeeded")
	}
	if err := s.Put("fox", "ring"); !errors.Is(err, first) {
		t.Errorf("second Put: got %v, want %v", err, first)
	}
	if err := s.Delete("cat"); !errors.Is(err, first) {
		t.Errorf("Delete: got %v, want %v", err, first)
	}
	if v, ok := s.Get("cat"); !ok || v != "meow" {
		t.Errorf("Get(cat) = %q, %v; want meow, true", v, ok)
	}
	if _, ok := s.Get("dog"); ok {
		t.Error("failed Put changed the map")
	}
}

func TestConcurrentWrites(t *testing.T) {
	s, dir := newStore(t)
	const writers, perWriter = 8, 100

	var wg sync.WaitGroup
	for w := range writers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range perWriter {
				key := fmt.Sprintf("w%d-%d", w, i)
				if err := s.Put(key, key); err != nil {
					t.Error(err)
					return
				}
				s.Get(key)
			}
		}()
	}
	wg.Wait()
	s.Close()

	s, cut := reopen(t, dir)
	defer s.Close()
	if cut != 0 || len(s.data) != writers*perWriter {
		t.Fatalf("after reopen: %d keys, cut %d; want %d keys, cut 0", len(s.data), cut, writers*perWriter)
	}
}

func TestStatus(t *testing.T) {
	s, _ := newStore(t)
	mustPut(t, s, "cat", "meow")
	mustPut(t, s, "dog", "woof")
	mustPut(t, s, "cat", "purr")
	if err := s.Delete("dog"); err != nil {
		t.Fatal(err)
	}
	if keys, broken := s.Status(); keys != 1 || broken != nil {
		t.Fatalf("Status() = %d, %v; want 1, nil", keys, broken)
	}

	s.log.Close()
	first := s.Put("fox", "ring")
	if keys, broken := s.Status(); keys != 1 || !errors.Is(broken, first) {
		t.Errorf("after a failed write Status() = %d, %v; want 1, %v", keys, broken, first)
	}
}
