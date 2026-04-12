package runner

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"syscall"
	"time"
	"unsafe"

	"crossfuzz/pkg/coverage"
	"crossfuzz/pkg/protocol"
)

// TimeoutError is returned by Execute when the target doesn't respond within the deadline.
type TimeoutError struct {
	TargetName string
}

func (e *TimeoutError) Error() string {
	return fmt.Sprintf("target %s: execution timed out", e.TargetName)
}

// CrashError is returned by Execute when the target process terminates unexpectedly.
type CrashError struct {
	TargetName string
	ExitState  *os.ProcessState // may be nil
}

func (e *CrashError) Error() string {
	if e.ExitState != nil {
		return fmt.Sprintf("target %s: process crashed (%v)", e.TargetName, e.ExitState)
	}
	return fmt.Sprintf("target %s: process crashed", e.TargetName)
}

// ProcessConfig configures a persistent-mode target process.
type ProcessConfig struct {
	Name          string
	Binary        string
	Args          []string
	Env           []string
	Timeout       time.Duration
	MemLimitBytes uint64 // 0 = no limit; sets RLIMIT_AS on the child via prlimit64
}

// Process manages a single target process communicating via pipes + shared memory.
type Process struct {
	cfg   ProcessConfig
	cmd   *exec.Cmd
	cmdW  io.WriteCloser // coordinator -> worker commands
	respR io.ReadCloser  // worker -> coordinator responses
	shm   *coverage.SharedMem
}

// NewProcess creates a new process runner. Call Start() to launch it.
func NewProcess(cfg ProcessConfig) (*Process, error) {
	shm, err := coverage.Create()
	if err != nil {
		return nil, fmt.Errorf("create shared memory: %w", err)
	}
	return &Process{cfg: cfg, shm: shm}, nil
}

func (p *Process) Name() string    { return p.cfg.Name }
func (p *Process) SHMPath() string { return p.shm.Path() }

// Start launches the target process and waits for its "ready" handshake.
func (p *Process) Start() error {
	// Create pipes: coordinator writes to cmdW, worker reads from cmdR (fd 3).
	// Worker writes to respW (fd 4), coordinator reads from respR.
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

	p.cmdW = cmdW
	p.respR = respR

	p.cmd = exec.Command(p.cfg.Binary, p.cfg.Args...)
	p.cmd.Stdout = os.Stdout
	p.cmd.Stderr = os.Stderr
	p.cmd.ExtraFiles = []*os.File{cmdR, respW} // fd 3, fd 4
	// Place the child in its own process group so that terminal signals
	// (e.g. Ctrl+C / SIGINT) do not propagate to it. The coordinator owns
	// the child's lifecycle and kills it explicitly via Stop().
	p.cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	env := append(os.Environ(), fmt.Sprintf("CROSSFUZZ_SHM=%s", p.shm.Path()))
	env = append(env, p.cfg.Env...)
	p.cmd.Env = env

	if err := p.cmd.Start(); err != nil {
		cmdR.Close()
		cmdW.Close()
		respR.Close()
		respW.Close()
		return fmt.Errorf("start process %s (%s): %w", p.cfg.Name, p.cfg.Binary, err)
	}

	// Apply per-target virtual-memory limit via prlimit64.
	// This races with early allocations in the target's init code, but is
	// sufficient to catch runaway allocators once the target is running.
	if p.cfg.MemLimitBytes > 0 {
		lim := syscall.Rlimit{Cur: p.cfg.MemLimitBytes, Max: p.cfg.MemLimitBytes}
		syscall.Syscall6(syscall.SYS_PRLIMIT64,
			uintptr(p.cmd.Process.Pid),
			uintptr(syscall.RLIMIT_AS),
			uintptr(unsafe.Pointer(&lim)),
			0, 0, 0)
	}

	// Close the child-side ends in the parent.
	cmdR.Close()
	respW.Close()

	// Wait for the worker's ready handshake.
	msg, err := protocol.Decode(p.respR)
	if err != nil {
		p.Stop()
		return fmt.Errorf("wait for ready from %s: %w", p.cfg.Name, err)
	}
	if msg.Type != protocol.TypeReady {
		p.Stop()
		return fmt.Errorf("expected ready from %s, got %s", p.cfg.Name, msg.Type)
	}

	return nil
}

type decodeResult struct {
	msg *protocol.Message
	err error
}

// Execute runs one fuzz iteration: write input, send command, read result.
// If the target doesn't respond within the configured timeout, it is killed and
// restarted; a *TimeoutError is returned. If the target crashes (pipe closes
// unexpectedly or non-zero exit), it is restarted and a *CrashError is returned.
func (p *Process) Execute(input []byte) ([]byte, []byte, error) {
	p.shm.WriteInput(input)
	p.shm.SetStatus(coverage.StatusOK)
	p.shm.ResetCoverage()

	if err := protocol.Encode(p.cmdW, &protocol.Message{
		Type:      protocol.TypeFuzz,
		TimeoutMS: int(p.cfg.Timeout.Milliseconds()),
	}); err != nil {
		// Pipe write failed — process probably died before we could send.
		state, restartErr := p.reapAndRestart()
		if restartErr != nil {
			return nil, nil, fmt.Errorf("restart %s after crash: %w", p.cfg.Name, restartErr)
		}
		return nil, nil, &CrashError{TargetName: p.cfg.Name, ExitState: state}
	}

	// Read the response in a background goroutine so the timeout can fire
	// concurrently. The channel is buffered so the goroutine never leaks —
	// closing p.respR (done in reapAndRestart) unblocks the Decode and the
	// goroutine can send to the channel even if nobody reads it.
	ch := make(chan decodeResult, 1)
	go func() {
		msg, err := protocol.Decode(p.respR)
		ch <- decodeResult{msg, err}
	}()

	select {
	case res := <-ch:
		if res.err != nil {
			state, restartErr := p.reapAndRestart()
			if restartErr != nil {
				return nil, nil, fmt.Errorf("restart %s after crash: %w", p.cfg.Name, restartErr)
			}
			return nil, nil, &CrashError{TargetName: p.cfg.Name, ExitState: state}
		}
		if res.msg.Error != "" {
			return nil, nil, fmt.Errorf("target %s error: %s", p.cfg.Name, res.msg.Error)
		}
		output := p.shm.ReadOutput()
		cov := make([]byte, coverage.BitmapSize)
		copy(cov, p.shm.Coverage())
		return output, cov, nil

	case <-time.After(p.cfg.Timeout):
		// Kill the process so the background goroutine's Decode unblocks,
		// allowing it to send to the buffered channel and exit cleanly.
		if p.cmd != nil && p.cmd.Process != nil {
			p.cmd.Process.Kill()
		}
		_, restartErr := p.reapAndRestart()
		if restartErr != nil {
			return nil, nil, fmt.Errorf("restart %s after timeout: %w", p.cfg.Name, restartErr)
		}
		return nil, nil, &TimeoutError{TargetName: p.cfg.Name}
	}
}

// reapAndRestart kills (if not already dead) and reaps the current process,
// closes the communication pipes, then starts a fresh process reusing the
// existing shared-memory region. Returns the exit state of the dead process.
func (p *Process) reapAndRestart() (*os.ProcessState, error) {
	// Close write pipe — if the process is still alive this sends it EOF.
	if p.cmdW != nil {
		p.cmdW.Close()
		p.cmdW = nil
	}
	// Close read pipe — this unblocks the background decode goroutine (if any).
	if p.respR != nil {
		p.respR.Close()
		p.respR = nil
	}
	// Reap the process.
	var state *os.ProcessState
	if p.cmd != nil {
		p.cmd.Wait()
		state = p.cmd.ProcessState
		p.cmd = nil
	}
	// Restart using the same shared-memory region.
	if err := p.Start(); err != nil {
		return state, err
	}
	return state, nil
}

// Stop terminates the target process and cleans up all resources.
func (p *Process) Stop() error {
	if p.cmdW != nil {
		// Best-effort: ask the target to shut down cleanly. Ignore write
		// errors — the process may have already exited (e.g. on campaign end).
		_ = protocol.Encode(p.cmdW, &protocol.Message{Type: protocol.TypeShutdown})
		p.cmdW.Close()
		p.cmdW = nil
	}
	if p.respR != nil {
		p.respR.Close()
		p.respR = nil
	}
	if p.cmd != nil && p.cmd.Process != nil {
		p.cmd.Process.Kill()
		p.cmd.Wait()
	}
	if p.shm != nil {
		p.shm.Close()
		p.shm.Remove()
		p.shm = nil
	}
	return nil
}
