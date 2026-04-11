package main

import (
	"fmt"
	"strings"
)

const (
	ansiReset = "\033[0m"
	ansiDiff  = "\033[1;31m" // bold red
)

// diffMask returns a boolean slice of length max(len(outputs...)) where true
// means that byte position differs across at least two outputs.
// Missing bytes (outputs shorter than the longest) are treated as differing.
func diffMask(outputs [][]byte) []bool {
	maxLen := 0
	for _, o := range outputs {
		if len(o) > maxLen {
			maxLen = len(o)
		}
	}
	if maxLen == 0 {
		return nil
	}
	mask := make([]bool, maxLen)
	for i := 0; i < maxLen; i++ {
		set := false
		ref := -1
		for _, o := range outputs {
			if i >= len(o) {
				set = true
				break
			}
			if ref == -1 {
				ref = int(o[i])
			} else if int(o[i]) != ref {
				set = true
				break
			}
		}
		mask[i] = set
	}
	return mask
}

// colorHexDump renders data in the same format as hex.Dump but applies bold-red
// ANSI highlighting to any byte position where mask[i] is true.
// mask may be nil or shorter than data; unmasked positions are rendered normally.
func colorHexDump(data []byte, mask []bool) string {
	if len(data) == 0 {
		return ""
	}
	var sb strings.Builder
	for i := 0; i < len(data); i += 16 {
		end := i + 16
		if end > len(data) {
			end = len(data)
		}
		chunk := data[i:end]

		// Offset
		fmt.Fprintf(&sb, "%08x  ", i)

		// Hex bytes
		for j, b := range chunk {
			if j == 8 {
				sb.WriteByte(' ')
			}
			pos := i + j
			if pos < len(mask) && mask[pos] {
				fmt.Fprintf(&sb, "%s%02x%s ", ansiDiff, b, ansiReset)
			} else {
				fmt.Fprintf(&sb, "%02x ", b)
			}
		}

		// Padding to align the ASCII column when the line is short
		if len(chunk) < 16 {
			missing := 16 - len(chunk)
			pad := missing * 3
			if len(chunk) <= 8 {
				pad++ // the mid-gap space was never printed
			}
			sb.WriteString(strings.Repeat(" ", pad))
		}

		// ASCII column
		sb.WriteString(" |")
		for j, b := range chunk {
			pos := i + j
			c := b
			if c < 32 || c > 126 {
				c = '.'
			}
			if pos < len(mask) && mask[pos] {
				fmt.Fprintf(&sb, "%s%c%s", ansiDiff, c, ansiReset)
			} else {
				sb.WriteByte(c)
			}
		}
		sb.WriteString("|\n")
	}
	return sb.String()
}
