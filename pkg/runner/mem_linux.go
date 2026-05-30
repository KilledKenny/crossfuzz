//go:build linux

package runner

import (
	"syscall"
	"unsafe"
)

func applyMemLimit(pid int, bytes uint64) {
	lim := syscall.Rlimit{Cur: bytes, Max: bytes}
	syscall.Syscall6(syscall.SYS_PRLIMIT64,
		uintptr(pid),
		uintptr(syscall.RLIMIT_AS),
		uintptr(unsafe.Pointer(&lim)),
		0, 0, 0)
}
