package runner

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sync"
	"syscall"

	"github.com/KilledKenny/crossfuzz/pkg/protocol"
)

// CompareProcess manages a persistent comparator harness process.
// The comparer reads outputs directly from each target's shared memory region
// and reports whether all outputs match.
type CompareProcess struct {
	cfg        ProcessConfig
	targetSHMs map[string]string // target name -> SHM file path
	cmd        *exec.Cmd
	cmdW       io.WriteCloser
	respR      io.ReadCloser
	mu         sync.Mutex
}

// NewCompareProcess creates a comparator runner. The targetSHMs map provides
// the shared memory path for each fuzz target so the comparer can read their
// outputs directly. Call Start() to launch the process.
func NewCompareProcess(cfg ProcessConfig, targetSHMs map[string]string) (*CompareProcess, error) {
	return &CompareProcess{cfg: cfg, targetSHMs: targetSHMs}, nil
}

// Start launches the comparator process and waits for its ready handshake.
// The process receives CROSSFUZZ_SHM_TARGETS as a JSON-encoded map of
// target name to SHM file path.
func (c *CompareProcess) Start() error {
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

	c.cmdW = cmdW
	c.respR = respR

	targetsJSON, err := json.Marshal(c.targetSHMs)
	if err != nil {
		cmdR.Close()
		cmdW.Close()
		respR.Close()
		respW.Close()
		return fmt.Errorf("marshal target SHM map: %w", err)
	}

	c.cmd = exec.Command(c.cfg.Binary, c.cfg.Args...)
	c.cmd.Stdout = os.Stdout
	c.cmd.Stderr = os.Stderr
	c.cmd.ExtraFiles = []*os.File{cmdR, respW} // fd 3, fd 4
	c.cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	env := append(os.Environ(), fmt.Sprintf("CROSSFUZZ_SHM_TARGETS=%s", targetsJSON))
	env = append(env, c.cfg.Env...)
	c.cmd.Env = env

	if err := c.cmd.Start(); err != nil {
		cmdR.Close()
		cmdW.Close()
		respR.Close()
		respW.Close()
		return fmt.Errorf("start compare process %s (%s): %w", c.cfg.Name, c.cfg.Binary, err)
	}

	cmdR.Close()
	respW.Close()

	msg, err := protocol.Decode(c.respR)
	if err != nil {
		c.Stop()
		return fmt.Errorf("wait for ready from comparator %s: %w", c.cfg.Name, err)
	}
	if msg.Type != protocol.TypeReady {
		c.Stop()
		return fmt.Errorf("expected ready from comparator %s, got %s", c.cfg.Name, msg.Type)
	}
	return nil
}

// Compare sends a compare command listing which targets to compare and returns
// the mismatch description. An empty string means all outputs match.
func (c *CompareProcess) Compare(targets []string) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if err := protocol.Encode(c.cmdW, &protocol.Message{
		Type:    protocol.TypeCompare,
		Targets: targets,
	}); err != nil {
		return "", fmt.Errorf("send compare command: %w", err)
	}

	msg, err := protocol.Decode(c.respR)
	if err != nil {
		return "", fmt.Errorf("read compare result: %w", err)
	}
	if msg.Type != protocol.TypeCompareResult {
		return "", fmt.Errorf("expected compare_result, got %s", msg.Type)
	}
	return msg.Error, nil
}

// Stop shuts down the comparator process and releases all resources.
func (c *CompareProcess) Stop() error {
	if c.cmdW != nil {
		_ = protocol.Encode(c.cmdW, &protocol.Message{Type: protocol.TypeShutdown})
		c.cmdW.Close()
		c.cmdW = nil
	}
	if c.respR != nil {
		c.respR.Close()
		c.respR = nil
	}
	if c.cmd != nil && c.cmd.Process != nil {
		c.cmd.Process.Kill()
		c.cmd.Wait()
	}
	return nil
}
