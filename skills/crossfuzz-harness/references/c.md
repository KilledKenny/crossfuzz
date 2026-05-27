# C / C++ Harness

## Harness files

To verify crossfuzz is installed: `pkg-config --modversion crossfuzz-c`

Not installed? See [c-install.md](c-install.md).

## C target

```c
#include <crossfuzz/crossfuzz.h>
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
#include <crossfuzz/crossfuzz.hpp>
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

### CMake (recommended)

```cmake
find_package(crossfuzz REQUIRED)

add_executable(my_target my_target.c)
target_link_libraries(my_target PRIVATE crossfuzz::c)
```

```cmake
find_package(crossfuzz REQUIRED)

add_executable(my_target my_target.cpp)
target_link_libraries(my_target PRIVATE crossfuzz::cpp)
```

Linking against `crossfuzz::c` / `crossfuzz::cpp` automatically propagates
`-fsanitize-coverage=trace-pc-guard` to the target.

```sh
cmake -B build -DCMAKE_C_COMPILER=clang -DCMAKE_BUILD_TYPE=Release
cmake --build build
```

### pkg-config / manual

```sh
# C
clang $(pkg-config --cflags crossfuzz-c) -o my_target my_target.c \
  $(pkg-config --libs crossfuzz-c)

# C++
clang++ -std=c++23 $(pkg-config --cflags crossfuzz-cpp) -o my_target my_target.cpp \
  $(pkg-config --libs crossfuzz-cpp)
```

`-fsanitize-coverage=trace-pc-guard` is **required** for coverage. Without it the binary runs but produces no coverage signal. Both cmake and pkg-config add it automatically.

## TOML config entry

```toml
# C target
[[target]]
name = "c_impl"
language = "c"
binary = "./build/my_target"
build_cmd = "cmake -B build -DCMAKE_C_COMPILER=clang -DCMAKE_BUILD_TYPE=Release && cmake --build build"

# C++ target
[[target]]
name = "cpp_impl"
language = "cpp"
binary = "./build/my_target"
build_cmd = "cmake -B build -DCMAKE_CXX_COMPILER=clang++ -DCMAKE_BUILD_TYPE=Release && cmake --build build"
```

## Settings

Pass a `crossfuzz_settings_t` as the second argument; pass `NULL` for defaults.

```c
#include <crossfuzz/crossfuzz.h>

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
#include <crossfuzz/crossfuzz.h>
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
#include <crossfuzz/crossfuzz.h>
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
