package engine

import "encoding/binary"

var interesting8 = []byte{0, 1, 2, 0x7f, 0x80, 0xfe, 0xff}

var interesting16 = []uint16{0, 1, 0x7f, 0x80, 0xff, 0x100, 0x7fff, 0x8000, 0xffff}

var interesting32 = []uint32{0, 1, 0x7f, 0x80, 0xff, 0x100, 0x7fff, 0x8000, 0xffff, 0x10000, 0x7fffffff, 0x80000000, 0xffffffff}

func (m *Mutator) bitFlip(data []byte) []byte {
	if len(data) == 0 {
		return data
	}
	pos := m.rng.Intn(len(data) * 8)
	data[pos/8] ^= 1 << (uint(pos) % 8)
	return data
}

func (m *Mutator) byteFlip(data []byte) []byte {
	if len(data) == 0 {
		return data
	}
	data[m.rng.Intn(len(data))] ^= 0xff
	return data
}

func (m *Mutator) arithmeticInc(data []byte) []byte {
	if len(data) == 0 {
		return data
	}
	pos := m.rng.Intn(len(data))
	data[pos] += byte(m.rng.Intn(35) + 1)
	return data
}

func (m *Mutator) arithmeticDec(data []byte) []byte {
	if len(data) == 0 {
		return data
	}
	pos := m.rng.Intn(len(data))
	data[pos] -= byte(m.rng.Intn(35) + 1)
	return data
}

func (m *Mutator) interestingValue(data []byte) []byte {
	if len(data) == 0 {
		return data
	}
	pos := m.rng.Intn(len(data))
	remaining := len(data) - pos
	switch {
	case remaining >= 4 && m.rng.Intn(3) == 0:
		v := interesting32[m.rng.Intn(len(interesting32))]
		if m.rng.Intn(2) == 0 {
			binary.LittleEndian.PutUint32(data[pos:], v)
		} else {
			binary.BigEndian.PutUint32(data[pos:], v)
		}
	case remaining >= 2 && m.rng.Intn(2) == 0:
		v := interesting16[m.rng.Intn(len(interesting16))]
		if m.rng.Intn(2) == 0 {
			binary.LittleEndian.PutUint16(data[pos:], v)
		} else {
			binary.BigEndian.PutUint16(data[pos:], v)
		}
	default:
		data[pos] = interesting8[m.rng.Intn(len(interesting8))]
	}
	return data
}

func (m *Mutator) randomByte(data []byte) []byte {
	if len(data) == 0 {
		return data
	}
	data[m.rng.Intn(len(data))] = byte(m.rng.Intn(256))
	return data
}

func (m *Mutator) insertBytes(data []byte) []byte {
	n := m.rng.Intn(4) + 1
	pos := m.rng.Intn(len(data) + 1)
	inserted := make([]byte, n)
	for i := range inserted {
		inserted[i] = byte(m.rng.Intn(256))
	}
	result := make([]byte, 0, len(data)+n)
	result = append(result, data[:pos]...)
	result = append(result, inserted...)
	result = append(result, data[pos:]...)
	return result
}

func (m *Mutator) deleteBytes(data []byte) []byte {
	if len(data) <= 1 {
		return data
	}
	n := m.rng.Intn(4) + 1
	if n >= len(data) {
		n = len(data) - 1
	}
	pos := m.rng.Intn(len(data) - n + 1)
	result := make([]byte, 0, len(data)-n)
	result = append(result, data[:pos]...)
	result = append(result, data[pos+n:]...)
	return result
}

func (m *Mutator) duplicateChunk(data []byte) []byte {
	if len(data) < 2 {
		return data
	}
	maxChunk := len(data) / 2
	if maxChunk > 32 {
		maxChunk = 32
	}
	chunkSize := m.rng.Intn(maxChunk) + 1
	srcPos := m.rng.Intn(len(data) - chunkSize + 1)
	dstPos := m.rng.Intn(len(data) + 1)

	chunk := make([]byte, chunkSize)
	copy(chunk, data[srcPos:srcPos+chunkSize])

	result := make([]byte, 0, len(data)+chunkSize)
	result = append(result, data[:dstPos]...)
	result = append(result, chunk...)
	result = append(result, data[dstPos:]...)
	return result
}
