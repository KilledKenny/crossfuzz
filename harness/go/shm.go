package crossfuzz

import (
	"fmt"
	"os"

	"crossfuzz/pkg/coverage"
)

// OpenSHM opens and maps the shared memory region identified by the
// CROSSFUZZ_SHM environment variable. Returns an error if the variable
// is not set or the file cannot be mapped.
func OpenSHM() (*coverage.SharedMem, error) {
	return OpenSHMPath(os.Getenv("CROSSFUZZ_SHM"))
}

// OpenSHMPath opens and maps the shared memory file at the given path.
func OpenSHMPath(path string) (*coverage.SharedMem, error) {
	if path == "" {
		return nil, fmt.Errorf("crossfuzz: CROSSFUZZ_SHM not set")
	}
	return coverage.Open(path)
}

// SetStatus writes a status code to the shared memory header.
func SetStatus(shm *coverage.SharedMem, status uint32) {
	shm.SetStatus(status)
}
