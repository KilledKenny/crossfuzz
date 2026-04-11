package runner

import (
	"fmt"
	"os"
	"os/exec"

	"crossfuzz/pkg/coverage"
)

// ServerProcess manages a long-running server target that communicates only
// via shared memory. Unlike Process, it has no pipe handshake — the server
// starts up independently and the coordinator reads its coverage bitmap
// after each harness execution.
type ServerProcess struct {
	name   string
	binary string
	args   []string
	env    []string
	cmd    *exec.Cmd
	shm    *coverage.SharedMem
}

// NewServerProcess creates a server runner. Call Start() to launch it.
func NewServerProcess(cfg ProcessConfig) (*ServerProcess, error) {
	shm, err := coverage.Create()
	if err != nil {
		return nil, fmt.Errorf("create shared memory: %w", err)
	}
	return &ServerProcess{
		name:   cfg.Name,
		binary: cfg.Binary,
		args:   cfg.Args,
		env:    cfg.Env,
		shm:    shm,
	}, nil
}

func (s *ServerProcess) Name() string { return s.name }

// Start launches the server process with CROSSFUZZ_SHM set. No pipe handshake.
func (s *ServerProcess) Start() error {
	s.cmd = exec.Command(s.binary, s.args...)
	s.cmd.Stdout = os.Stdout
	s.cmd.Stderr = os.Stderr

	env := append(os.Environ(), fmt.Sprintf("CROSSFUZZ_SHM=%s", s.shm.Path()))
	env = append(env, s.env...)
	s.cmd.Env = env

	if err := s.cmd.Start(); err != nil {
		return fmt.Errorf("start server process %s (%s): %w", s.name, s.binary, err)
	}
	return nil
}

// Execute reads the current coverage bitmap from shared memory. The server
// updates its bitmap continuously as it handles requests; the coordinator
// calls this after the harness has finished an iteration.
func (s *ServerProcess) Execute(_ []byte) ([]byte, []byte, error) {
	cov := make([]byte, coverage.BitmapSize)
	copy(cov, s.shm.Coverage())
	return nil, cov, nil
}

// ResetCoverage zeroes the server's coverage bitmap in shared memory.
// The coordinator calls this before each harness execution so only
// coverage from the current iteration is captured.
func (s *ServerProcess) ResetCoverage() {
	s.shm.ResetCoverage()
}

// Stop kills the server process and releases shared memory.
func (s *ServerProcess) Stop() error {
	if s.cmd != nil && s.cmd.Process != nil {
		s.cmd.Process.Kill()
		s.cmd.Wait()
	}
	if s.shm != nil {
		s.shm.Close()
		s.shm.Remove()
		s.shm = nil
	}
	return nil
}
