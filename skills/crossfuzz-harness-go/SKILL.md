---
name: crossfuzz-harness-go
description: Use this skill when the user is writing a Go target for cross_fuzz, needs to know the Go harness API, wants to know which build flags are required for coverage, or needs to set up coverpkg filtering. Trigger for questions like "how do I write a Go target?", "what build flags do I need for Go?", "why isn't my Go target producing coverage?", "how do I use coverpkg?", "how does the Go harness work?", or "how do I write a Go filter or comparator?".
---

# Go Harness

## Package path

```
crossfuzz/harness/go
```

Import as `crossfuzz "crossfuzz/harness/go"` in your target's `main` package.

## Fuzz target

```go
package main

import (
    "encoding/base64"
    crossfuzz "crossfuzz/harness/go"
)

func target(data []byte) ([]byte, error) {
    encoded := base64.StdEncoding.EncodeToString(data)
    return []byte(encoded), nil
}

func main() {
    crossfuzz.Fuzz(target)
}
```

### Function signature

```go
func Fuzz(target func([]byte) ([]byte, error), opts ...Settings)
```

The target receives input bytes and returns output bytes. Return a non-nil error to mark execution as error-status (the comparator still runs but sees the error).

## Build command (required flags)

```bash
go build -cover -covermode=atomic -coverpkg="$PKGS" -o my_target_bin .
```

`-cover -covermode=atomic` is **required** — without it the binary produces no coverage.

`-coverpkg` controls which packages are instrumented. **Do not use `-coverpkg=./...`** — it misses third-party packages your target calls into. Instead, filter the full dependency list:

```bash
PKGS=$(go list -deps . | grep -vE '^(runtime$|runtime/.*|sync$|sync/.*|internal/.*|reflect$|syscall$|os$|os/.*)' | paste -sd,)
go build -cover -covermode=atomic -coverpkg="$PKGS" -o ../my_target_bin .
```

This includes stdlib packages your code actually delegates into (e.g. `encoding/json`, `net/url`) while excluding noisy runtime internals.

## TOML config entry

```toml
[[target]]
name = "go_impl"
language = "go"
binary = "./go_target_bin"
build_cmd = "cd go_target && PKGS=$(go list -deps . | grep -vE '^(runtime$|runtime/.*|sync$|sync/.*|internal/.*|reflect$|syscall$|os$|os/.*)' | paste -sd,) && go build -cover -covermode=atomic -coverpkg=\"$PKGS\" -o ../go_target_bin ."
```

For Settings, Filter, Compare, and server mode APIs read `<skill-dir>/references/go-harness.md`.
