#ifndef CROSSFUZZ_H
#define CROSSFUZZ_H

#include <stdint.h>
#include <stddef.h>

/*
 * User-implemented fuzz target.
 *
 * data/size: the fuzz input.
 * out:       buffer to write output into (up to 1 MB).
 * out_size:  set to the number of bytes written to out.
 *
 * Return 0 on success, non-zero on error.
 */
int crossfuzz_target(const uint8_t *data, size_t size,
                     uint8_t *out, size_t *out_size);

/*
 * Start the harness loop. Call from main().
 * This enters persistent mode and processes inputs until shutdown.
 */
int crossfuzz_run(void);

#endif /* CROSSFUZZ_H */
