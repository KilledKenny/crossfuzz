package coverage

// BitmapSize is the size of the coverage bitmap in bytes (64 KB).
const BitmapSize = 1 << 16

// HasNewBits reports whether current contains any coverage not yet in global.
func HasNewBits(global, current []byte) bool {
	for i := 0; i < len(global) && i < len(current); i++ {
		if current[i]&^global[i] != 0 {
			return true
		}
	}
	return false
}

// Merge ORs src into dst.
func Merge(dst, src []byte) {
	for i := 0; i < len(dst) && i < len(src); i++ {
		dst[i] |= src[i]
	}
}

// CountBits returns the number of non-zero bytes (edges hit) in the bitmap.
func CountBits(bitmap []byte) int {
	count := 0
	for _, b := range bitmap {
		if b != 0 {
			count++
		}
	}
	return count
}

// Reset zeroes the entire bitmap.
func Reset(bitmap []byte) {
	for i := range bitmap {
		bitmap[i] = 0
	}
}

// Bucketize rounds each counter down to the nearest power of two.
// This groups hit counts into coarse buckets (1, 2, 4, 8, 16, 32, 64, 128).
func Bucketize(bitmap []byte) {
	for i, v := range bitmap {
		if v == 0 {
			continue
		}
		v |= v >> 1
		v |= v >> 2
		v |= v >> 4
		bitmap[i] = v - (v >> 1)
	}
}
