package coverage

import (
	"encoding/binary"
	"fmt"
	"os"
	"syscall"
)

// Shared memory layout.
const (
	HeaderSize         = 64
	InputRegionOffset  = HeaderSize
	InputRegionSize    = 1 << 20 // 1 MB
	OutputRegionOffset = InputRegionOffset + InputRegionSize
	OutputRegionSize   = 1 << 20 // 1 MB
	CoverageOffset     = OutputRegionOffset + OutputRegionSize
	TotalShmSize       = CoverageOffset + BitmapSize // ~2 MB + 64 KB
)

// Header field offsets within the first 64 bytes.
const (
	OffExecCount = 0  // uint64
	OffInputLen  = 8  // uint32
	OffOutputLen = 12 // uint32
	OffStatus    = 16 // uint32
)

// Status values.
const (
	StatusOK    uint32 = 0
	StatusError uint32 = 1
	StatusCrash uint32 = 2
)

// SharedMem is a memory-mapped region shared between coordinator and a target.
type SharedMem struct {
	data []byte
	file *os.File
	path string
}

// Create allocates a new shared memory region backed by a temp file.
func Create() (*SharedMem, error) {
	f, err := os.CreateTemp("", "crossfuzz-shm-*")
	if err != nil {
		return nil, fmt.Errorf("create temp file: %w", err)
	}
	if err := f.Truncate(int64(TotalShmSize)); err != nil {
		f.Close()
		os.Remove(f.Name())
		return nil, fmt.Errorf("truncate: %w", err)
	}
	data, err := syscall.Mmap(int(f.Fd()), 0, TotalShmSize,
		syscall.PROT_READ|syscall.PROT_WRITE, syscall.MAP_SHARED)
	if err != nil {
		f.Close()
		os.Remove(f.Name())
		return nil, fmt.Errorf("mmap: %w", err)
	}
	return &SharedMem{data: data, file: f, path: f.Name()}, nil
}

// Open maps an existing shared memory file.
func Open(path string) (*SharedMem, error) {
	f, err := os.OpenFile(path, os.O_RDWR, 0)
	if err != nil {
		return nil, fmt.Errorf("open shm file: %w", err)
	}
	data, err := syscall.Mmap(int(f.Fd()), 0, TotalShmSize,
		syscall.PROT_READ|syscall.PROT_WRITE, syscall.MAP_SHARED)
	if err != nil {
		f.Close()
		return nil, fmt.Errorf("mmap: %w", err)
	}
	return &SharedMem{data: data, file: f, path: path}, nil
}

// Path returns the filesystem path to the backing file.
func (s *SharedMem) Path() string { return s.path }

func (s *SharedMem) InputLen() uint32 {
	return binary.LittleEndian.Uint32(s.data[OffInputLen:])
}

func (s *SharedMem) SetInputLen(n uint32) {
	binary.LittleEndian.PutUint32(s.data[OffInputLen:], n)
}

func (s *SharedMem) OutputLen() uint32 {
	return binary.LittleEndian.Uint32(s.data[OffOutputLen:])
}

func (s *SharedMem) SetOutputLen(n uint32) {
	binary.LittleEndian.PutUint32(s.data[OffOutputLen:], n)
}

func (s *SharedMem) Status() uint32 {
	return binary.LittleEndian.Uint32(s.data[OffStatus:])
}

func (s *SharedMem) SetStatus(v uint32) {
	binary.LittleEndian.PutUint32(s.data[OffStatus:], v)
}

// WriteInput copies data into the input region.
func (s *SharedMem) WriteInput(data []byte) {
	n := len(data)
	if n > InputRegionSize {
		n = InputRegionSize
	}
	copy(s.data[InputRegionOffset:], data[:n])
	s.SetInputLen(uint32(n))
}

// ReadInput returns a copy of the input data.
func (s *SharedMem) ReadInput() []byte {
	n := int(s.InputLen())
	if n > InputRegionSize {
		n = InputRegionSize
	}
	out := make([]byte, n)
	copy(out, s.data[InputRegionOffset:InputRegionOffset+n])
	return out
}

// WriteOutput copies data into the output region.
func (s *SharedMem) WriteOutput(data []byte) {
	n := len(data)
	if n > OutputRegionSize {
		n = OutputRegionSize
	}
	copy(s.data[OutputRegionOffset:], data[:n])
	s.SetOutputLen(uint32(n))
}

// ReadOutput returns a copy of the output data.
func (s *SharedMem) ReadOutput() []byte {
	n := int(s.OutputLen())
	if n > OutputRegionSize {
		n = OutputRegionSize
	}
	out := make([]byte, n)
	copy(out, s.data[OutputRegionOffset:OutputRegionOffset+n])
	return out
}

// Coverage returns a direct slice into the mmap'd coverage bitmap.
func (s *SharedMem) Coverage() []byte {
	return s.data[CoverageOffset : CoverageOffset+BitmapSize]
}

// ResetCoverage zeroes the coverage bitmap.
func (s *SharedMem) ResetCoverage() {
	Reset(s.Coverage())
}

// Close unmaps and closes the shared memory file.
func (s *SharedMem) Close() error {
	if s.data != nil {
		syscall.Munmap(s.data)
		s.data = nil
	}
	if s.file != nil {
		s.file.Close()
		s.file = nil
	}
	return nil
}

// Remove deletes the backing file.
func (s *SharedMem) Remove() error {
	return os.Remove(s.path)
}
