#include "crossfuzz.h"
#include <stdlib.h>
#include <string.h>

/* abort() when the first input byte is 'X', producing a crash finding once
 * the fuzzer mutates an input to begin with that byte. */
static int target(const uint8_t *data, size_t size,
                  uint8_t *out, size_t *out_size)
{
    if (size > 0 && data[0] == 'X') {
        abort();
    }
    memcpy(out, data, size);
    *out_size = size;
    return 0;
}

int main(void) { return crossfuzz_fuzz(target, NULL); }
