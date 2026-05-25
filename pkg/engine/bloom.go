package engine

import "hash/fnv"

// bloom is a fixed-size Bloom filter used to skip inputs that have already
// been sent to the targets. False positives waste the occasional novel input;
// false negatives are impossible. Sized at ~1 Mbit (128 KB) per worker, which
// at three hashes per item keeps the false-positive rate under ~1% until
// roughly 100k unique inputs have been seen.
type bloom struct {
	bits []uint64
	mask uint64
}

const (
	bloomBits = 1 << 20 // 1 Mbit
)

func newBloom() *bloom {
	return &bloom{
		bits: make([]uint64, bloomBits/64),
		mask: bloomBits - 1,
	}
}

func (b *bloom) hashes(data []byte) (uint64, uint64, uint64) {
	h := fnv.New64a()
	h.Write(data)
	h1 := h.Sum64()
	// Two cheap derived hashes — double-hashing trick.
	h2 := h1*0x9E3779B97F4A7C15 + 0xBF58476D1CE4E5B9
	h3 := h2*0x94D049BB133111EB + 0xC6BC279692B5C323
	return h1 & b.mask, h2 & b.mask, h3 & b.mask
}

// CheckAndAdd returns true if data was probably seen before. Either way the
// data's hashes are now set.
func (b *bloom) CheckAndAdd(data []byte) bool {
	h1, h2, h3 := b.hashes(data)
	a := b.bits[h1>>6] & (uint64(1) << (h1 & 63))
	c := b.bits[h2>>6] & (uint64(1) << (h2 & 63))
	d := b.bits[h3>>6] & (uint64(1) << (h3 & 63))
	seen := a != 0 && c != 0 && d != 0
	b.bits[h1>>6] |= uint64(1) << (h1 & 63)
	b.bits[h2>>6] |= uint64(1) << (h2 & 63)
	b.bits[h3>>6] |= uint64(1) << (h3 & 63)
	return seen
}
