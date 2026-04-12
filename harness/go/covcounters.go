package crossfuzz

// Parser for the binary stream produced by runtime/coverage.WriteCounters.
//
// Format reference: internal/coverage/defs.go and
// internal/coverage/decodecounter/decodecounterfile.go in the Go source.
//
// Layout:
//   CounterFileHeader      (32 bytes)
//     Magic     [4]byte    {0x00, 0x63, 0x77, 0x6d}
//     Version   uint32     currently 1
//     MetaHash  [16]byte   MD5 of the companion covmeta file
//     CFlavor   uint8      1=CtrRaw (fixed uint32)  2=CtrULeb128 (varint)
//     BigEndian uint8      0 on all supported platforms
//     _         [6]byte    padding
//   1..N Segments
//     SegmentHeader (16 bytes)
//       FcnEntries uint64
//       StrTabLen  uint32
//       ArgsLen    uint32
//     StrTab    [StrTabLen]byte  (skipped — we don't need names)
//     ArgsPayload [ArgsLen]byte  (skipped — OS args / GOOS / GOARCH)
//     padding to 4-byte alignment
//     FuncPayload * FcnEntries
//       NumCtrs   uint32|ULEB
//       PkgId     uint32|ULEB
//       FuncId    uint32|ULEB
//       Counters[NumCtrs] uint32|ULEB
//   CounterFileFooter (16 bytes, trailing, ignored)
//
// For an in-process WriteCounters call there is exactly one segment.

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
)

const (
	covHeaderSize  = 32
	covSegHdrSize  = 16
	covFlavorRaw   = 1
	covFlavorULeb  = 2
	covFileVersion = 1
)

var covMagic = [4]byte{0x00, 0x63, 0x77, 0x6d}

// funcCounters is one function's counter payload from a WriteCounters snapshot.
type funcCounters struct {
	pkgID    uint32
	funcID   uint32
	counters []uint32 // aliases into covReader.ctrBuf; do not retain
}

// covReader parses covcounters binary streams. Buffers are reused across
// calls so that repeated parses do not allocate once steady state is reached.
type covReader struct {
	scratch []funcCounters
	ctrBuf  []uint32
}

// parse decodes data and returns the function payloads found in its
// (first) segment. The returned slice aliases r.scratch and each
// funcCounters.counters aliases r.ctrBuf; both are invalidated by the
// next call to parse.
func (r *covReader) parse(data []byte) ([]funcCounters, error) {
	flavor, bigEndian, rest, err := r.readHeader(data)
	if err != nil {
		return nil, err
	}

	// WriteCounters writes exactly one segment per call. We parse that
	// segment and stop; any trailing footer is ignored.
	headerConsumed := len(data) - len(rest)
	fcnEntries, payload, err := r.readSegment(rest, headerConsumed)
	if err != nil {
		return nil, err
	}

	r.scratch = r.scratch[:0]
	r.ctrBuf = r.ctrBuf[:0]

	pos := 0
	for i := uint64(0); i < fcnEntries; i++ {
		numCtrs, pkgID, funcID, n, err := r.readFuncHeader(payload[pos:], flavor, bigEndian)
		if err != nil {
			return nil, fmt.Errorf("func %d header: %w", i, err)
		}
		pos += n

		start := len(r.ctrBuf)
		for j := uint32(0); j < numCtrs; j++ {
			val, n, err := r.readCounter(payload[pos:], flavor, bigEndian)
			if err != nil {
				return nil, fmt.Errorf("func %d counter %d: %w", i, j, err)
			}
			pos += n
			r.ctrBuf = append(r.ctrBuf, val)
		}

		r.scratch = append(r.scratch, funcCounters{
			pkgID:    pkgID,
			funcID:   funcID,
			counters: r.ctrBuf[start : start+int(numCtrs)],
		})
	}

	return r.scratch, nil
}

func (r *covReader) readHeader(data []byte) (flavor uint8, bigEndian bool, rest []byte, err error) {
	if len(data) < covHeaderSize {
		return 0, false, nil, io.ErrUnexpectedEOF
	}
	if data[0] != covMagic[0] || data[1] != covMagic[1] ||
		data[2] != covMagic[2] || data[3] != covMagic[3] {
		return 0, false, nil, errors.New("covcounters: bad magic")
	}
	version := binary.LittleEndian.Uint32(data[4:8])
	if version > covFileVersion {
		return 0, false, nil, fmt.Errorf("covcounters: unsupported version %d", version)
	}
	// data[8:24] = MetaHash (ignored)
	flavor = data[24]
	if flavor != covFlavorRaw && flavor != covFlavorULeb {
		return 0, false, nil, fmt.Errorf("covcounters: unknown flavor %d", flavor)
	}
	bigEndian = data[25] != 0
	// data[26:32] = padding
	return flavor, bigEndian, data[covHeaderSize:], nil
}

// readSegment parses one segment header and returns its function count plus
// the byte slice containing the function payloads. absOffset is the current
// file-relative byte offset of `data[0]` (used for 4-byte alignment
// calculation, which is file-relative per the Go reference implementation).
func (r *covReader) readSegment(data []byte, absOffset int) (fcnEntries uint64, payload []byte, err error) {
	if len(data) < covSegHdrSize {
		return 0, nil, fmt.Errorf("covcounters: segment header: %w", io.ErrUnexpectedEOF)
	}
	fcnEntries = binary.LittleEndian.Uint64(data[0:8])
	strTabLen := binary.LittleEndian.Uint32(data[8:12])
	argsLen := binary.LittleEndian.Uint32(data[12:16])

	pos := covSegHdrSize
	// Skip string table.
	if len(data)-pos < int(strTabLen) {
		return 0, nil, fmt.Errorf("covcounters: strtab: %w", io.ErrUnexpectedEOF)
	}
	pos += int(strTabLen)

	// Skip args payload.
	if len(data)-pos < int(argsLen) {
		return 0, nil, fmt.Errorf("covcounters: args: %w", io.ErrUnexpectedEOF)
	}
	pos += int(argsLen)

	// Align to 4-byte boundary, file-relative.
	fileOff := absOffset + pos
	if rem := fileOff % 4; rem != 0 {
		skip := 4 - rem
		if len(data)-pos < skip {
			return 0, nil, fmt.Errorf("covcounters: pad: %w", io.ErrUnexpectedEOF)
		}
		pos += skip
	}

	return fcnEntries, data[pos:], nil
}

func (r *covReader) readFuncHeader(data []byte, flavor uint8, bigEndian bool) (numCtrs, pkgID, funcID uint32, n int, err error) {
	var v uint64
	var k int

	v, k, err = readWord(data, flavor, bigEndian)
	if err != nil {
		return 0, 0, 0, 0, err
	}
	numCtrs = uint32(v)
	n += k

	v, k, err = readWord(data[n:], flavor, bigEndian)
	if err != nil {
		return 0, 0, 0, 0, err
	}
	pkgID = uint32(v)
	n += k

	v, k, err = readWord(data[n:], flavor, bigEndian)
	if err != nil {
		return 0, 0, 0, 0, err
	}
	funcID = uint32(v)
	n += k

	return numCtrs, pkgID, funcID, n, nil
}

func (r *covReader) readCounter(data []byte, flavor uint8, bigEndian bool) (uint32, int, error) {
	v, n, err := readWord(data, flavor, bigEndian)
	if err != nil {
		return 0, 0, err
	}
	return uint32(v), n, nil
}

// readWord reads one encoded word (uint32 for CtrRaw, ULEB128 for CtrULeb128).
func readWord(data []byte, flavor uint8, bigEndian bool) (uint64, int, error) {
	if flavor == covFlavorRaw {
		if len(data) < 4 {
			return 0, 0, io.ErrUnexpectedEOF
		}
		if bigEndian {
			return uint64(binary.BigEndian.Uint32(data)), 4, nil
		}
		return uint64(binary.LittleEndian.Uint32(data)), 4, nil
	}
	// CtrULeb128
	var result uint64
	var shift uint
	for i := 0; i < 10 && i < len(data); i++ {
		b := data[i]
		result |= uint64(b&0x7F) << shift
		if b&0x80 == 0 {
			return result, i + 1, nil
		}
		shift += 7
	}
	if len(data) < 10 {
		return 0, 0, io.ErrUnexpectedEOF
	}
	return 0, 0, errors.New("covcounters: ULEB128 overflow")
}
