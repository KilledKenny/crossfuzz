#include "crossfuzz.h"
#include <stdlib.h>
#include <string.h>

/* Allocates a 512 MiB slab when the input starts with 'M'. Under --max-memory
 * tighter than that the malloc fails and we abort, producing a crash finding.
 * Other inputs return immediately so the campaign progresses quickly. */
static int target(const uint8_t *data, size_t size,
                  uint8_t *out, size_t *out_size)
{
    if (size > 0 && data[0] == 'M') {
        size_t n = (size_t)512 * 1024 * 1024;
        void *p = malloc(n);
        if (!p) {
            abort();
        }
        /* Touch the head + tail to force the kernel to back the pages. */
        ((volatile char *)p)[0] = 1;
        ((volatile char *)p)[n - 1] = 1;
        free(p);
    }
    memcpy(out, data, size);
    *out_size = size;
    return 0;
}

int main(void) { return crossfuzz_fuzz(target, NULL); }
