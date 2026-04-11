package compare

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os/exec"
)

// Custom runs a user-supplied script to compare outputs.
//
// The script receives a JSON object on stdin with the shape:
//
//	{
//	  "input":   "<base64-encoded input>",
//	  "outputs": { "<target-name>": "<base64-encoded output>", ... }
//	}
//
// Exit code 0 means no discrepancy; any other exit code signals a finding.
// If the script writes a non-empty line to stdout it is used as the
// discrepancy description; otherwise a generic message is generated.
type Custom struct {
	// Script is the path (or shell command) to execute.
	Script string
}

func (c Custom) Name() string { return "custom" }

func (c Custom) Compare(input []byte, outputs map[string][]byte) *Discrepancy {
	payload := map[string]any{
		"input":   input,
		"outputs": outputs,
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return &Discrepancy{
			Input:       input,
			Outputs:     copyOutputs(outputs),
			Description: fmt.Sprintf("custom comparator: failed to marshal payload: %v", err),
			Comparator:  "custom",
		}
	}

	cmd := exec.Command("sh", "-c", c.Script)
	cmd.Stdin = bytes.NewReader(data)
	out, err := cmd.Output()

	if err == nil {
		// Exit 0 → no discrepancy.
		return nil
	}

	description := string(bytes.TrimSpace(out))
	if description == "" {
		description = fmt.Sprintf("custom comparator script %q exited with: %v", c.Script, err)
	}
	return &Discrepancy{
		Input:       input,
		Outputs:     copyOutputs(outputs),
		Description: description,
		Comparator:  "custom",
	}
}
