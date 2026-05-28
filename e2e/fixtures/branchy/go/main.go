package main

import (
	"fmt"

	"github.com/KilledKenny/crossfuzz/harness/go"
)

// target has a deliberately broad branch surface: ~16 branches per byte plus
// length-dependent branches. The e2e coverage_test asserts the fuzzer
// discovers many of these via mutation.
func target(data []byte) ([]byte, error) {
	var state int
	for _, b := range data {
		switch b & 0x0F {
		case 0x0:
			state += 1
		case 0x1:
			state += 2
		case 0x2:
			state += 3
		case 0x3:
			state += 5
		case 0x4:
			state += 7
		case 0x5:
			state += 11
		case 0x6:
			state += 13
		case 0x7:
			state += 17
		case 0x8:
			state -= 1
		case 0x9:
			state -= 2
		case 0xA:
			state -= 3
		case 0xB:
			state -= 5
		case 0xC:
			state ^= 0xAA
		case 0xD:
			state ^= 0x55
		case 0xE:
			state <<= 1
		case 0xF:
			state >>= 1
		}
	}
	if len(data) > 4 {
		state *= 2
	}
	if len(data) > 16 {
		state *= 3
	}
	return []byte(fmt.Sprintf("%d", state)), nil
}

func main() {
	crossfuzz.Fuzz(target)
}
