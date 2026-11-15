package store

import (
	"bytes"
	"encoding/hex"
	"errors"
	"io"
	"strings"
	"testing"
)

func TestEntryKnownBytes(t *testing.T) {
	got := hex.EncodeToString(encodeEntry(entry{op: opPut, key: "cat", value: "meow"}))
	want := "0103000000636174040000006d656f770b06ec26"
	if got != want {
		t.Fatalf("got %s, want %s", got, want)
	}
}

func TestEntryRoundTrip(t *testing.T) {
	entries := []entry{
		{op: opPut, key: "cat", value: "meow"},
		{op: opDelete, key: "cat"},
		{op: opPut, key: "empty", value: ""},
		{op: opPut, key: "poem", value: "roses\nare=red"},
		{op: opPut, key: strings.Repeat("k", maxKeyLen), value: strings.Repeat("v", maxValueLen)},
	}
	var file []byte
	for _, e := range entries {
		file = append(file, encodeEntry(e)...)
	}

	r := bytes.NewReader(file)
	for i, want := range entries {
		got, err := readEntry(r)
		if err != nil {
			t.Fatalf("entry %d: %v", i, err)
		}
		if got != want {
			t.Fatalf("entry %d: got %+v, want %+v", i, got, want)
		}
	}
	if _, err := readEntry(r); err != io.EOF {
		t.Fatalf("after the last entry: got %v, want io.EOF", err)
	}
}

func TestReadEntryCutOff(t *testing.T) {
	full := encodeEntry(entry{op: opPut, key: "cat", value: "meow"})
	if _, err := readEntry(bytes.NewReader(nil)); err != io.EOF {
		t.Fatalf("empty input: got %v, want io.EOF", err)
	}
	for n := 1; n < len(full); n++ {
		_, err := readEntry(bytes.NewReader(full[:n]))
		if !errors.Is(err, errIncompleteEntry) {
			t.Fatalf("first %d of %d bytes: got %v, want errIncompleteEntry", n, len(full), err)
		}
	}
}

func TestReadEntryDamaged(t *testing.T) {
	want := entry{op: opPut, key: "cat", value: "meow"}
	full := encodeEntry(want)
	for i := range full {
		damaged := bytes.Clone(full)
		damaged[i] ^= 0x01
		got, err := readEntry(bytes.NewReader(damaged))
		if err == nil {
			t.Fatalf("byte %d changed: read %+v without an error", i, got)
		}
	}
}

func TestReadEntryHugeLength(t *testing.T) {
	file := []byte{opPut, 0xff, 0xff, 0xff, 0xff}
	if _, err := readEntry(bytes.NewReader(file)); !errors.Is(err, errCorruptEntry) {
		t.Fatalf("got %v, want errCorruptEntry", err)
	}
}

func TestReadEntryUnknownOp(t *testing.T) {
	file := encodeEntry(entry{op: 7, key: "cat", value: "meow"})
	if _, err := readEntry(bytes.NewReader(file)); !errors.Is(err, errCorruptEntry) {
		t.Fatalf("got %v, want errCorruptEntry", err)
	}
}

func TestCheckHeader(t *testing.T) {
	if err := checkHeader(bytes.NewReader(encodeHeader())); err != nil {
		t.Fatalf("good header: %v", err)
	}
	for _, bad := range []string{"JUNK\x01", "KVLG\x02", "KV", ""} {
		if err := checkHeader(strings.NewReader(bad)); err == nil {
			t.Errorf("header %q accepted", bad)
		}
	}
}
