#include "crossfuzz.h"
#include <string.h>

/* Standard base64 alphabet. */
static const char B64[] =
    "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/";

int crossfuzz_target(const uint8_t *data, size_t size,
                     uint8_t *out, size_t *out_size)
{
    size_t out_len = 4 * ((size + 2) / 3);
    *out_size = out_len;

    size_t i, j;
    for (i = 0, j = 0; i < size; ) {
        uint32_t a = (i < size) ? data[i++] : 0;
        uint32_t b = (i < size) ? data[i++] : 0;
        uint32_t c = (i < size) ? data[i++] : 0;
        uint32_t triple = (a << 16) | (b << 8) | c;

        out[j++] = (uint8_t)B64[(triple >> 18) & 0x3F];
        out[j++] = (uint8_t)B64[(triple >> 12) & 0x3F];
        out[j++] = (uint8_t)B64[(triple >> 6)  & 0x3F];
        out[j++] = (uint8_t)B64[ triple        & 0x3F];
    }

    /* Padding. */
    size_t mod = size % 3;
    if (mod > 0) {
        out[out_len - 1] = '=';
        //if (mod == 1)
        //    out[out_len - 2] = '=';
    }

    return 0;
}

int main(void)
{
    return crossfuzz_run();
}
