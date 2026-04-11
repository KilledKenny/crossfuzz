package runner

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"sync"
	"syscall"

	"crossfuzz/pkg/coverage"
	"crossfuzz/pkg/protocol"
)

// FilterProcess manages a persistent input filter process.
// The coordinator writes each candidate input into shared memory, sends a
// filter message, and the filter process responds with accept or reject.
// A mutex serialises concurrent workers through the single filter process.
type FilterProcess struct {
	cfg   ProcessConfig
	cmd   *exec.Cmd
	cmdW  io.WriteCloser
	respR io.ReadCloser
	shm   *coverage.SharedMem
	mu    sync.Mutex
}

// NewFilterProcess creates a filter runner. Call Start() to launch it.
func NewFilterProcess(cfg ProcessConfig) (*FilterProcess, error) {
	shm, err := coverage.Create()
	if err != nil {
		return nil, fmt.Errorf("create shared memory for filter: %w", err)
	}
	return &FilterProcess{cfg: cfg, shm: shm}, nil
}

// Start launches the filter process and waits for its ready handshake.
func (f *FilterProcess) Start() error {
	cmdR, cmdW, err := os.Pipe()
	if err != nil {
		return fmt.Errorf("create command pipe: %w", err)
	}
	respR, respW, err := os.Pipe()
	if err != nil {
		cmdR.Close()
		cmdW.Close()
		return fmt.Errorf("create response pipe: %w", err)
	}

	f.cmdW = cmdW
	f.respR = respR

	f.cmd = exec.Command(f.cfg.Binary, f.cfg.Args...)
	f.cmd.Stdout = os.Stdout
	f.cmd.Stderr = os.Stderr
	f.cmd.ExtraFiles = []*os.File{cmdR, respW} // fd 3, fd 4
	f.cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	env := append(os.Environ(), fmt.Sprintf("CROSSFUZZ_SHM=%s", f.shm.Path()))
	env = append(env, f.cfg.Env...)
	f.cmd.Env = env

	if err := f.cmd.Start(); err != nil {
		cmdR.Close()
		cmdW.Close()
		respR.Close()
		respW.Close()
		return fmt.Errorf("start filter process %s (%s): %w", f.cfg.Name, f.cfg.Binary, err)
	}

	cmdR.Close()
	respW.Close()

	msg, err := protocol.Decode(f.respR)
	if err != nil {
		f.Stop()
		return fmt.Errorf("wait for ready from filter %s: %w", f.cfg.Name, err)
	}
	if msg.Type != protocol.TypeReady {
		f.Stop()
		return fmt.Errorf("expected ready from filter %s, got %s", f.cfg.Name, msg.Type)
	}
	return nil
}

// Filter writes input into shared memory, sends a filter command to the filter
// process, and returns true if the process accepts the input.
func (f *FilterProcess) Filter(input []byte) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.shm.WriteInput(input)

	if err := protocol.Encode(f.cmdW, &protocol.Message{Type: protocol.TypeFilter}); err != nil {
		return false, fmt.Errorf("send filter command: %w", err)
	}

	msg, err := protocol.Decode(f.respR)
	if err != nil {
		return false, fmt.Errorf("read filter result: %w", err)
	}
	if msg.Type != protocol.TypeFilterResult {
		return false, fmt.Errorf("expected filter_result, got %s", msg.Type)
	}
	return msg.OK, nil
}

// Stop shuts down the filter process and releases all resources.
func (f *FilterProcess) Stop() error {
	if f.cmdW != nil {
		_ = protocol.Encode(f.cmdW, &protocol.Message{Type: protocol.TypeShutdown})
		f.cmdW.Close()
		f.cmdW = nil
	}
	if f.respR != nil {
		f.respR.Close()
		f.respR = nil
	}
	if f.cmd != nil && f.cmd.Process != nil {
		f.cmd.Process.Kill()
		f.cmd.Wait()
	}
	if f.shm != nil {
		f.shm.Close()
		f.shm.Remove()
		f.shm = nil
	}
	return nil
}
