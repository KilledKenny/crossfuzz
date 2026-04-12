# Go Harness — Complete Reference

## Settings

```go
crossfuzz.Fuzz(target, crossfuzz.Settings{
    Instrument: true,   // default: true — set false when harness is thin HTTP client
    Warmup:     10,     // run this many warmup iterations to mask flaky coverage slots
    Transform:  false,  // filter mode: when true, returned bytes replace input
})
```

| Field | Default | Description |
|-------|---------|-------------|
| `Instrument` | `true` | Collect and report coverage via `runtime/coverage` |
| `Warmup` | `0` | Extra iterations on first input to mask GC/allocator coverage noise (harness internally defaults to 200 if warmup is enabled) |
| `Transform` | `false` | Filter mode only: returned bytes replace the original input |
| `Hinting` | `false` | Reserved |

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

Configure in `crossfuzz.toml` as `[input_filter]` (not as a `[[target]]`).

## Compare target

```go
package main

import crossfuzz "crossfuzz/harness/go"

func main() {
    crossfuzz.Compare(func(input []byte, names []string, outputs [][]byte) string {
        // Return "" for match, non-empty string for mismatch.
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

When the Go binary is a `type = "server"` target (long-running HTTP server), use the standalone functions:

```go
import crossfuzz "crossfuzz/harness/go"

func main() {
    // Call once during server initialization:
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

## Build command details

```bash
cd go_target
PKGS=$(go list -deps . | grep -vE '^(runtime$|runtime/.*|sync$|sync/.*|internal/.*|reflect$|syscall$|os$|os/.*)' | paste -sd,)
go build -cover -covermode=atomic -coverpkg="$PKGS" -o ../my_target_bin .
```

### Why the coverpkg filter?

`-coverpkg=./...` only instruments packages in your module. Third-party and stdlib packages your code calls into produce no coverage. The `go list -deps` filter includes all transitive dependencies except noisy runtime internals.

The regex excludes:
- `runtime`, `runtime/*` — extremely noisy; GC/scheduler activity contaminates coverage
- `sync`, `sync/*` — mutex/channel internals; noisy
- `internal/*` — compiler/runtime internals
- `reflect`, `syscall`, `os`, `os/*` — low signal, high noise

Adjust the exclusion list if you want coverage from specific stdlib packages.

### `-covermode=atomic`

Required for concurrent targets (most real code uses goroutines). `-covermode=count` races under parallel execution and corrupts counters.

## Full example: base64 (Go)

From `examples/base64/go_target/main.go`:

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

Build command from `examples/base64/crossfuzz.toml`:

```bash
cd go_target && PKGS=$(go list -deps . | grep -vE '^(runtime$|runtime/.*|sync$|sync/.*|internal/.*|reflect$|syscall$|os$|os/.*)' | paste -sd,) && go build -cover -covermode=atomic -coverpkg="$PKGS" -o ../go_target_bin .
```

## Common pitfalls

- **Missing `-cover`**: binary runs but produces no coverage — fuzzer never discovers new inputs.
- **Using `-coverpkg=./...`**: misses stdlib/third-party packages — you lose coverage from `encoding/json`, `net/url`, etc.
- **Using `-covermode=count`** with concurrent code: data races corrupt coverage counters.
- **`runtime/coverage` requires Go 1.20+**: check your Go version with `go version`.
- **Not calling `InitServer()`** in server mode: no coverage collected from server.
