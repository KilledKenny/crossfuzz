# Go Harness

## Package path

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

Return a non-nil error to mark execution as error-status (the comparator still runs but sees the error).

## Build command

```bash
cd go_target
PKGS=$(go list -deps . | grep -vE '^(runtime$|runtime/.*|sync$|sync/.*|internal/.*|reflect$|syscall$|os$|os/.*)' | paste -sd,)
go build -cover -covermode=atomic -coverpkg="$PKGS" -o ../my_target_bin .
```

`-cover -covermode=atomic` is **required** — without it the binary produces no coverage.

`-coverpkg=./...` only instruments packages in your module — third-party and stdlib packages your code calls into produce no coverage. The `go list -deps` filter above includes all transitive dependencies except noisy runtime internals.

The regex excludes:
- `runtime`, `runtime/*` — GC/scheduler activity contaminates coverage
- `sync`, `sync/*` — mutex/channel internals; noisy
- `internal/*` — compiler/runtime internals
- `reflect`, `syscall`, `os`, `os/*` — low signal, high noise

Adjust the exclusion list if you want coverage from specific stdlib packages.

`-covermode=atomic` is required for concurrent targets; `-covermode=count` races under parallel execution and corrupts counters.

## TOML config entry

```toml
[[target]]
name = "go_impl"
language = "go"
binary = "./go_target_bin"
build_cmd = "cd go_target && PKGS=$(go list -deps . | grep -vE '^(runtime$|runtime/.*|sync$|sync/.*|internal/.*|reflect$|syscall$|os$|os/.*)' | paste -sd,) && go build -cover -covermode=atomic -coverpkg=\"$PKGS\" -o ../go_target_bin ."
```

## Settings

```go
crossfuzz.Fuzz(target, crossfuzz.Settings{
    Instrument: true,   // default: true — set false when harness is a thin HTTP client
    Transform:  false,  // filter mode: when true, returned bytes replace input
})
```

| Field | Default | Description |
|-------|---------|-------------|
| `Instrument` | `true` | Collect and report coverage via `runtime/coverage` |
| `Transform` | `false` | Filter mode only: returned bytes replace the original input |

## Filter target

```go
package main

import crossfuzz "crossfuzz/harness/go"

func main() {
    crossfuzz.Filter(func(data []byte) ([]byte, bool) {
        // Return (output, true) to accept, (nil, false) to reject.
        // output is only used when Settings.Transform = true.
        if len(data) < 4 {
            return nil, false
        }
        return data, true
    })
}
```

Configure in `crossfuzz.toml` as `[input_filter]` (not `[[target]]`).

## Compare target

```go
package main

import (
    "fmt"
    crossfuzz "crossfuzz/harness/go"
)

func main() {
    crossfuzz.Compare(func(input []byte, names []string, outputs [][]byte) string {
        if len(outputs) < 2 {
            return ""
        }
        if string(outputs[0]) != string(outputs[1]) {
            return fmt.Sprintf("%s returned %q, %s returned %q",
                names[0], outputs[0], names[1], outputs[1])
        }
        return ""
    })
}
```

Configure in `crossfuzz.toml` as `[comparator] type = "harness"`.

## Server mode APIs

For `type = "server"` targets (long-running HTTP server):

```go
import crossfuzz "crossfuzz/harness/go"

func main() {
    if err := crossfuzz.InitServer(); err != nil {
        log.Fatal(err)
    }

    // Before handling each request:
    crossfuzz.ClearCoverage()

    // ... handle the request ...

    // After the request completes:
    crossfuzz.CollectCoverage()
}
```

`InitServer()` opens the SHM and starts instrumentation. It is a no-op if `CROSSFUZZ_SHM` is not set, so the binary can run standalone without the coordinator.

## Common pitfalls

- **Missing `-cover`**: binary runs but produces no coverage — fuzzer never discovers new inputs.
- **Using `-coverpkg=./...`**: misses stdlib/third-party packages.
- **Using `-covermode=count`** with concurrent code: data races corrupt coverage counters.
- **`runtime/coverage` requires Go 1.20+**.
- **Not calling `InitServer()`** in server mode: no coverage collected from server.
