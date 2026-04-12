---
name: crossfuzz-harness-c
description: Use this skill when the user is writing a C or C++ target for cross_fuzz, needs to know the function signature for a C harness, wants to know the build flags required for coverage, or is setting up a C/C++ fuzzing target. Trigger for questions like "how do I write a C target?", "what clang flags do I need?", "how do I use the C++ harness?", "my C target isn't producing coverage", or "how do I compile my C target for cross_fuzz?".
---

# C / C++ Harness

## Harness files

- `harness/c/crossfuzz.h` — header (include this in your target)
- `harness/c/crossfuzz.c` — implementation (compile alongside your target)
- `harness/cpp/crossfuzz.hpp` — C++ wrapper (optional, adds lambda support)

## C target

Implement one function, pass it to `crossfuzz_fuzz`, done.

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
    const uint8_t *data,   // input bytes
    size_t         size,   // input length
    uint8_t       *out,    // output buffer (1 MB available)
    size_t        *out_size // set this to the number of bytes you wrote
);
```

### Build command

```bash
clang -fsanitize-coverage=trace-pc-guard -O2 \
  -I ../../harness/c \
  -o my_target my_target.c ../../harness/c/crossfuzz.c
```

`-fsanitize-coverage=trace-pc-guard` is **required** for coverage. Without it the binary runs but produces no coverage signal.

## C++ target

Use `crossfuzz.hpp` for a lambda-friendly interface:

```cpp
#include "../../harness/cpp/crossfuzz.hpp"
#include <span>
#include <vector>

int main()
{
    return crossfuzz::fuzz([](std::span<const uint8_t> input) -> std::vector<uint8_t> {
        // Process input, return output as a vector.
        // Throw on error — the harness catches exceptions and sets status=error.
        return std::vector<uint8_t>(input.begin(), input.end());
    });
}
```

### C++ build command (two-step compile)

```bash
# Step 1: compile the C harness implementation
clang -fsanitize-coverage=trace-pc-guard -O2 \
  -c ../../harness/c/crossfuzz.c -o crossfuzz_c.o

# Step 2: compile and link your C++ target
clang++ -std=c++23 -fsanitize-coverage=trace-pc-guard -O2 \
  -I ../../harness/c \
  -o my_target my_target.cpp ../../harness/cpp/crossfuzz.cpp crossfuzz_c.o
```

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

For settings, filter, and compare variants with annotated examples, read `<skill-dir>/references/c-harness.md`.
