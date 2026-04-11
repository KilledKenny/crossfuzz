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
 * Optional settings for crossfuzz_run_ex().
 * Zero-initialise to get default behaviour.
 */
typedef struct {
    /*
     * When non-zero, coverage data is not written to the shared memory bitmap.
     * Use this when the harness is only a trigger and coverage should come
     * entirely from instrumented server targets.
     */
    int disable_instrumentation;
} crossfuzz_settings_t;

/*
 * Start the harness loop. Call from main().
 * This enters persistent mode and processes inputs until shutdown.
 */
int crossfuzz_run(void);

/*
 * Like crossfuzz_run() but accepts an optional settings struct.
 * Pass NULL to use defaults (equivalent to crossfuzz_run()).
 */
int crossfuzz_run_ex(const crossfuzz_settings_t *settings);

#endif /* CROSSFUZZ_H */
