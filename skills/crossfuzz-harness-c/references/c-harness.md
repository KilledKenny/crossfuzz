# C / C++ Harness — Complete Reference

## Settings

Pass a `crossfuzz_settings_t` as the second argument. Pass `NULL` for defaults.

```c
crossfuzz_settings_t s = crossfuzz_default_settings();
s.instrument = 0;  // disable coverage (use when harness is a thin HTTP client)
s.transform  = 1;  // filter mode: returned bytes replace the original input
return crossfuzz_fuzz(target, &s);
```

| Field | Default | Description |
|-------|---------|-------------|
| `instrument` | 1 | Enable SanitizerCoverage feedback |
| `warmup` | 0 | Reserved |
| `transform` | 0 | Filter mode: if 1, filter output replaces input for targets |
| `hinting` | 0 | Reserved |

## C++ Settings

```cpp
crossfuzz::Settings settings;
settings.disable_instrumentation = true;
return crossfuzz::fuzz(myLambda, settings);
```

## Filter target (C)

Use `crossfuzz_filter` to build an input filter process:

```c
#include "crossfuzz.h"
#include <string.h>

static int url_filter(const uint8_t *data, size_t size,
                      uint8_t *out, size_t *out_size,
                      int *accepted)
{
    // Accept only inputs that contain "://"
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

int main(void)
{
    return crossfuzz_filter(url_filter, NULL);
}
```

### Filter function signature

```c
typedef int (*crossfuzz_filter_fn)(
    const uint8_t *data,     // input bytes
    size_t         size,
    uint8_t       *out,      // transformed output (written if transform=1)
    size_t        *out_size,
    int           *accepted  // set to 1 to accept, 0 to reject
);
```

Configure in `crossfuzz.toml` as `[input_filter]` (not as a `[[target]]`).

## Compare target (C)

Use `crossfuzz_compare` to build a custom comparator process. It reads all target outputs from shared memory via `CROSSFUZZ_SHM_TARGETS`.

```c
#include "crossfuzz.h"
#include <string.h>

static const char *result_buf = NULL;

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

int main(void)
{
    return crossfuzz_compare(my_compare, NULL);
}
```

### Compare function signature

```c
typedef const char* (*crossfuzz_compare_fn)(
    const uint8_t  *input,
    size_t          input_size,
    int             num_targets,
    const char    **target_names,
    const uint8_t **target_outputs,
    const size_t   *target_output_sizes
);
// Return NULL or "" for match; any other string = mismatch message.
// The string must stay valid until the NEXT call to this function.
```

Configure in `crossfuzz.toml` as `[comparator] type = "harness"`.

## Standalone server-mode functions

When the C harness is a thin HTTP client (server target), call these manually:

```c
crossfuzz_open_shm();              // map CROSSFUZZ_SHM
crossfuzz_start_instrumentation(); // point coverage bitmap at SHM

// Before each request:
crossfuzz_clear_instrumentation();

// After the server responds:
crossfuzz_collect_instrumentation(); // no-op for C; coverage written directly by SanitizerCoverage
```

## Full example: base64 (C)

From `examples/base64/c_target.c`:

```c
#include "crossfuzz.h"

static const char B64[] =
    "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/";

static int target(const uint8_t *data, size_t size,
                  uint8_t *out, size_t *out_size)
{
    size_t out_len = 4 * ((size + 2) / 3);
    *out_size = out_len;
    size_t i = 0, j = 0;
    while (i < size) {
        uint32_t a = data[i++];
        uint32_t b = (i < size) ? data[i++] : 0;
        uint32_t c = (i < size) ? data[i++] : 0;
        uint32_t triple = (a << 16) | (b << 8) | c;
        out[j++] = (uint8_t)B64[(triple >> 18) & 0x3F];
        out[j++] = (uint8_t)B64[(triple >> 12) & 0x3F];
        out[j++] = (uint8_t)B64[(triple >> 6)  & 0x3F];
        out[j++] = (uint8_t)B64[ triple        & 0x3F];
    }
    size_t mod = size % 3;
    if (mod > 0) { out[out_len-1] = '='; if (mod==1) out[out_len-2] = '='; }
    return 0;
}

int main(void) { return crossfuzz_fuzz(target, NULL); }
```

## Common pitfalls

- **Missing `-fsanitize-coverage=trace-pc-guard`**: binary runs but emits zero coverage — the fuzzer never discovers new inputs.
- **C++ exception escaping the fuzz function**: wrap in try/catch; unhandled exceptions crash the process.
- **Writing more than 1 MB to `out`**: the output region is exactly 1 MB; writing past it corrupts shared memory.
- **Returning non-zero**: marks status=error. The comparator still runs but the output is flagged as an error result.
- **clang version**: use the same clang version for harness and target (e.g. `clang-19`).
