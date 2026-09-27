package placement

import (
	"encoding/binary"
	"hash/fnv"
)

func hash64(parts ...string) uint64 {
	h := fnv.New64a()
	var length [8]byte

	for _, part := range parts {
		binary.LittleEndian.PutUint64(length[:], uint64(len(part)))
		h.Write(length[:])
		h.Write([]byte(part))
	}

	return h.Sum64()
}
