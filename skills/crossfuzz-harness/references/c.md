# C / C++ Harness

## Harness files

- `harness/c/crossfuzz.h` — header (include this in your target)
- `harness/c/crossfuzz.c` — implementation (compile alongside your target)
- `harness/cpp/crossfuzz.hpp` — C++ wrapper (optional, adds lambda support)

## C target

```c
#include "crossfuzz.h"
#include <string.h>

static int target(const uint8_t *data, size_t size,
                  uint8_t *out, size_t *out_size)
{
    // Process `data` (size bytes), write result to `out`, set *out_size.
    // Return 0 on success, non-zero on error.
    // out can hold up to 1 MB.
    memcpy(out, data, size);
    *out_size = size;
    return 0;
}

int main(void)
{
    return crossfuzz_fuzz(target, NULL);  // NULL = use default settings
}
```

### Function signature

```c
typedef int (*crossfuzz_fuzz_fn)(
    const uint8_t *data,    // input bytes
    size_t         size,    // input length
    uint8_t       *out,     // output buffer (1 MB available)
    size_t        *out_size // set this to the number of bytes you wrote
);
```

## C++ target

Use `crossfuzz.hpp` for a lambda-friendly interface:

```cpp
#include "../../harness/cpp/crossfuzz.hpp"
#include <span>
#include <vector>

int main()
{
    return crossfuzz::fuzz([](std::span<const uint8_t> input) -> std::vector<uint8_t> {
        // Throw on error — the harness catches exceptions and sets status=error.
        return std::vector<uint8_t>(input.begin(), input.end());
    });
}
```

## Build commands

### C

```bash
clang -fsanitize-coverage=trace-pc-guard -O2 \
  -I ../../harness/c \
  -o my_target my_target.c ../../harness/c/crossfuzz.c
```

### C++ (two-step compile)

```bash
clang -fsanitize-coverage=trace-pc-guard -O2 \
  -c ../../harness/c/crossfuzz.c -o crossfuzz_c.o

clang++ -std=c++23 -fsanitize-coverage=trace-pc-guard -O2 \
  -I ../../harness/c \
  -o my_target my_target.cpp ../../harness/cpp/crossfuzz.cpp crossfuzz_c.o
```

`-fsanitize-coverage=trace-pc-guard` is **required** for coverage. Without it the binary runs but produces no coverage signal.

## TOML config entry

```toml
# C target
[[target]]
name = "c_impl"
language = "c"
binary = "./c_target"
build_cmd = "clang -fsanitize-coverage=trace-pc-guard -O2 -I ../../harness/c -o c_target c_target.c ../../harness/c/crossfuzz.c"

# C++ target
[[target]]
name = "cpp_impl"
language = "cpp"
binary = "./cpp_target"
build_cmd = "clang -fsanitize-coverage=trace-pc-guard -O2 -c ../../harness/c/crossfuzz.c -o crossfuzz_c.o && clang++ -std=c++23 -fsanitize-coverage=trace-pc-guard -O2 -I ../../harness/c -o cpp_target cpp_target.cpp ../../harness/cpp/crossfuzz.cpp crossfuzz_c.o && rm crossfuzz_c.o"
```

## Settings

Pass a `crossfuzz_settings_t` as the second argument; pass `NULL` for defaults.

```c
crossfuzz_settings_t s = crossfuzz_default_settings();
s.instrument = 0;  // disable coverage (use when harness is a thin HTTP client)
s.transform  = 1;  // filter mode: returned bytes replace the original input
return crossfuzz_fuzz(target, &s);
```

C++:

```cpp
crossfuzz::Settings settings;
settings.instrument = false;
return crossfuzz::fuzz(myLambda, settings);
```

| Field | Default | Description |
|-------|---------|-------------|
| `instrument` | 1 / `true` | Enable SanitizerCoverage feedback |
| `transform` | 0 / `false` | Filter mode: if true, filter output replaces input for targets |

## Filter target (C)

```c
#include "crossfuzz.h"
#include <string.h>

static int url_filter(const uint8_t *data, size_t size,
                      uint8_t *out, size_t *out_size,
                      int *accepted)
{
    for (size_t i = 0; i + 2 < size; i++) {
        if (data[i] == ':' && data[i+1] == '/' && data[i+2] == '/') {
            *accepted = 1;
            memcpy(out, data, size);
            *out_size = size;
            return 0;
        }
    }
    *accepted = 0;
    *out_size = 0;
    return 0;
}

int main(void) { return crossfuzz_filter(url_filter, NULL); }
```

Configure in `crossfuzz.toml` as `[input_filter]` (not `[[target]]`).

## Compare target (C)

```c
#include "crossfuzz.h"
#include <string.h>

static const char *my_compare(const uint8_t *input, size_t input_size,
                               int num_targets,
                               const char **target_names,
                               const uint8_t **target_outputs,
                               const size_t *target_output_sizes)
{
    if (num_targets < 2) return NULL;
    if (target_output_sizes[0] != target_output_sizes[1] ||
        memcmp(target_outputs[0], target_outputs[1], target_output_sizes[0]) != 0) {
        return "outputs differ";  // must remain valid until next call
    }
    return NULL;  // NULL or "" = match
}

int main(void) { return crossfuzz_compare(my_compare, NULL); }
```

Configure in `crossfuzz.toml` as `[comparator] type = "harness"`.

## Server-mode functions

When the C harness is a thin HTTP client (server target), call manually:

```c
crossfuzz_open_shm();              // map CROSSFUZZ_SHM
crossfuzz_start_instrumentation(); // point coverage bitmap at SHM

// Before each request:
crossfuzz_clear_instrumentation();

// After the server responds:
crossfuzz_collect_instrumentation();
```

## Common pitfalls

- **Missing `-fsanitize-coverage=trace-pc-guard`**: binary runs but emits zero coverage.
- **C++ exception escaping the fuzz function**: wrap in try/catch; unhandled exceptions crash the process.
- **Writing more than 1 MB to `out`**: output region is exactly 1 MB; writing past corrupts shared memory.
- **Returning non-zero**: marks status=error. Comparator still runs but output is flagged.
- **clang version mismatch**: use the same clang version for harness and target.
