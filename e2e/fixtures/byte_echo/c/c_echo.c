#include "crossfuzz.h"
#include <string.h>

/* Echoes input unchanged. The per-byte category branches exist solely so the
 * fuzzer has edges to discover for the e2e path-discovery assertion. */
static int target(const uint8_t *data, size_t size,
                  uint8_t *out, size_t *out_size)
{
    for (size_t i = 0; i < size; i++) {
        uint8_t b = data[i];
        if (b < 0x20)      out[i] = b;
        else if (b < 0x40) out[i] = b;
        else if (b < 0x60) out[i] = b;
        else if (b < 0x80) out[i] = b;
        else               out[i] = b;
    }
    *out_size = size;
    return 0;
}

int main(void) { return crossfuzz_fuzz(target, NULL); }
