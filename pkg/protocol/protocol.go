package protocol

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
)

// Message types.
const (
	TypePing       = "ping"
	TypePong       = "pong"
	TypeFuzz       = "fuzz"
	TypeFuzzResult = "fuzz_result"
	TypeShutdown   = "shutdown"
	TypeReady      = "ready"
)

// Message is the wire protocol message exchanged between coordinator and workers.
type Message struct {
	Type      string `json:"type"`
	TimeoutMS int    `json:"timeout_ms,omitempty"`
	OK        bool   `json:"ok,omitempty"`
	Error     string `json:"error,omitempty"`
	ExecNS    int64  `json:"exec_ns,omitempty"`
}

// Encode writes a length-prefixed JSON message to w.
func Encode(w io.Writer, msg *Message) error {
	data, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("marshal message: %w", err)
	}
	var lenBuf [4]byte
	binary.BigEndian.PutUint32(lenBuf[:], uint32(len(data)))
	if _, err := w.Write(lenBuf[:]); err != nil {
		return fmt.Errorf("write length: %w", err)
	}
	if _, err := w.Write(data); err != nil {
		return fmt.Errorf("write payload: %w", err)
	}
	return nil
}

// Decode reads a length-prefixed JSON message from r.
func Decode(r io.Reader) (*Message, error) {
	var lenBuf [4]byte
	if _, err := io.ReadFull(r, lenBuf[:]); err != nil {
		return nil, fmt.Errorf("read length: %w", err)
	}
	length := binary.BigEndian.Uint32(lenBuf[:])
	if length > 1<<20 {
		return nil, fmt.Errorf("message too large: %d bytes", length)
	}
	data := make([]byte, length)
	if _, err := io.ReadFull(r, data); err != nil {
		return nil, fmt.Errorf("read payload: %w", err)
	}
	var msg Message
	if err := json.Unmarshal(data, &msg); err != nil {
		return nil, fmt.Errorf("unmarshal message: %w", err)
	}
	return &msg, nil
}
