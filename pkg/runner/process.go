package runner

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"time"

	"crossfuzz/pkg/coverage"
	"crossfuzz/pkg/protocol"
)

// ProcessConfig configures a persistent-mode target process.
type ProcessConfig struct {
	Name    string
	Binary  string
	Args    []string
	Env     []string
	Timeout time.Duration
}

// Process manages a single target process communicating via pipes + shared memory.
type Process struct {
	name    string
	binary  string
	args    []string
	env     []string
	timeout time.Duration

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
	return &Process{
		name:    cfg.Name,
		binary:  cfg.Binary,
		args:    cfg.Args,
		env:     cfg.Env,
		timeout: cfg.Timeout,
		shm:     shm,
	}, nil
}

func (p *Process) Name() string { return p.name }

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

	p.cmd = exec.Command(p.binary, p.args...)
	p.cmd.Stdout = os.Stdout
	p.cmd.Stderr = os.Stderr
	p.cmd.ExtraFiles = []*os.File{cmdR, respW} // fd 3, fd 4

	env := append(os.Environ(), fmt.Sprintf("CROSSFUZZ_SHM=%s", p.shm.Path()))
	env = append(env, p.env...)
	p.cmd.Env = env

	if err := p.cmd.Start(); err != nil {
		cmdR.Close()
		cmdW.Close()
		respR.Close()
		respW.Close()
		return fmt.Errorf("start process %s (%s): %w", p.name, p.binary, err)
	}

	// Close the child-side ends in the parent.
	cmdR.Close()
	respW.Close()

	// Wait for the worker's ready handshake.
	msg, err := protocol.Decode(p.respR)
	if err != nil {
		p.Stop()
		return fmt.Errorf("wait for ready from %s: %w", p.name, err)
	}
	if msg.Type != protocol.TypeReady {
		p.Stop()
		return fmt.Errorf("expected ready from %s, got %s", p.name, msg.Type)
	}

	return nil
}

// Execute runs one fuzz iteration: write input, send command, read result.
func (p *Process) Execute(input []byte) ([]byte, []byte, error) {
	p.shm.WriteInput(input)
	p.shm.SetStatus(coverage.StatusOK)
	p.shm.ResetCoverage()

	if err := protocol.Encode(p.cmdW, &protocol.Message{
		Type:      protocol.TypeFuzz,
		TimeoutMS: int(p.timeout.Milliseconds()),
	}); err != nil {
		return nil, nil, fmt.Errorf("send fuzz to %s: %w", p.name, err)
	}

	resp, err := protocol.Decode(p.respR)
	if err != nil {
		return nil, nil, fmt.Errorf("read response from %s: %w", p.name, err)
	}
	if resp.Error != "" {
		return nil, nil, fmt.Errorf("target %s error: %s", p.name, resp.Error)
	}

	output := p.shm.ReadOutput()
	cov := make([]byte, coverage.BitmapSize)
	copy(cov, p.shm.Coverage())

	return output, cov, nil
}

// Stop terminates the target process and cleans up.
func (p *Process) Stop() error {
	if p.cmdW != nil {
		if err := protocol.Encode(p.cmdW, &protocol.Message{Type: protocol.TypeShutdown}); err != nil {
			fmt.Fprintf(os.Stderr, "crossfuzz: send shutdown to %s: %v\n", p.name, err)
		}
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
