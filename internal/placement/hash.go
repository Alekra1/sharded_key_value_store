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

	return fmix64(h.Sum64())
}

// fmix64 is the 64-bit finalizer from MurmurHash3. It spreads every input bit
// across the whole output, which plain FNV-1a does not do for its last bytes.
func fmix64(k uint64) uint64 {
	k ^= k >> 33
	k *= 0xff51afd7ed558ccd
	k ^= k >> 33
	k *= 0xc4ceb9fe1a85ec53
	k ^= k >> 33
	return k
}
