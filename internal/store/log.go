package store

import (
	"encoding/binary"
	"errors"
	"fmt"
	"hash/crc32"
	"io"
)

// These values are stored in log files, so changing one needs a new logVersion.
const (
	opPut    byte = 1
	opDelete byte = 2

	// 256 doesn't fit in 1 byte and 65,536 doesn't fit in 2, so lengths are stored in 4.
	maxKeyLen   = 256
	maxValueLen = 64 << 10

	logMagic   = "KVLG"
	logVersion = 1
)

var (
	errIncompleteEntry = errors.New("log entry is incomplete")
	errCorruptEntry    = errors.New("log entry is corrupt")
)

type entry struct {
	op    byte
	key   string
	value string
}

func encodeHeader() []byte {
	return append([]byte(logMagic), logVersion)
}

func checkHeader(r io.Reader) error {
	var header [5]byte // magic (4) + version (1)
	if _, err := io.ReadFull(r, header[:]); err != nil {
		return fmt.Errorf("reading log header: %w", err)
	}
	magic := string(header[:4])
	version := header[4]
	if magic != logMagic {
		return errors.New("not a log file")
	}
	if version != logVersion {
		return fmt.Errorf("unsupported log version %d", version)
	}
	return nil
}

func encodeEntry(e entry) []byte {
	// operation (1) + key length (4) + key + value length (4) + value + checksum (4)
	buf := make([]byte, 0, 1+4+len(e.key)+4+len(e.value)+4)
	buf = append(buf, e.op)
	buf = binary.LittleEndian.AppendUint32(buf, uint32(len(e.key)))
	buf = append(buf, e.key...)
	buf = binary.LittleEndian.AppendUint32(buf, uint32(len(e.value)))
	buf = append(buf, e.value...)
	checksum := crc32.ChecksumIEEE(buf)
	return binary.LittleEndian.AppendUint32(buf, checksum)
}

// readEntry returns io.EOF only when r ends exactly between two entries.
func readEntry(r io.Reader) (entry, error) {
	sum := crc32.NewIEEE()
	in := io.TeeReader(r, sum)

	var op [1]byte
	if _, err := io.ReadFull(in, op[:]); err != nil {
		return entry{}, err
	}
	key, err := readField(in, maxKeyLen)
	if err != nil {
		return entry{}, err
	}
	value, err := readField(in, maxValueLen)
	if err != nil {
		return entry{}, err
	}

	want := sum.Sum32()
	// Read from r so the stored checksum isn't added to sum.
	stored, err := readUint32(r)
	if err != nil {
		return entry{}, err
	}
	if stored != want {
		return entry{}, errCorruptEntry
	}
	if op[0] != opPut && op[0] != opDelete {
		return entry{}, errCorruptEntry
	}
	return entry{op: op[0], key: string(key), value: string(value)}, nil
}

func readField(r io.Reader, limit uint32) ([]byte, error) {
	length, err := readUint32(r)
	if err != nil {
		return nil, err
	}
	if length > limit {
		return nil, errCorruptEntry
	}
	field := make([]byte, length)
	if _, err := io.ReadFull(r, field); err != nil {
		return nil, incomplete(err)
	}
	return field, nil
}

func readUint32(r io.Reader) (uint32, error) {
	var b [4]byte
	if _, err := io.ReadFull(r, b[:]); err != nil {
		return 0, incomplete(err)
	}
	return binary.LittleEndian.Uint32(b[:]), nil
}

// After a crash mid-write, io.ReadFull returns io.EOF or io.ErrUnexpectedEOF
// depending on how many bytes it got.
func incomplete(err error) error {
	if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
		return errIncompleteEntry
	}
	return err
}
