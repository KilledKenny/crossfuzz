//go:build !linux

package runner

import (
	"log"
	"sync"
)

var warnMemLimit sync.Once

func applyMemLimit(pid int, bytes uint64) {
	warnMemLimit.Do(func() {
		log.Println("warning: mem_limit_bytes is not supported on this platform (Linux only), ignoring")
	})
}
