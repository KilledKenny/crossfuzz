#ifndef CROSSFUZZ_H
#define CROSSFUZZ_H

#include <stdint.h>
#include <stddef.h>

/* ---- Settings ---- */

typedef struct {
    int instrument;  /* 1 (default) = enable coverage, 0 = disable */
    int warmup;      /* number of warmup iterations (default 0) */
    int transform;   /* 1 = filter may transform inputs (default 0) */
    int hinting;     /* reserved for future use (default 0) */
} crossfuzz_settings_t;

/* Returns a settings struct with recommended defaults. */
static inline crossfuzz_settings_t crossfuzz_default_settings(void)
{
    crossfuzz_settings_t s;
    s.instrument = 1;
    s.warmup = 0;
    s.transform = 0;
    s.hinting = 0;
    return s;
}

/* ---- Target callback types ---- */

/*
 * Fuzz target: receives input data/size, writes output to out/out_size.
 * Return 0 on success, non-zero on error.
 */
typedef int (*crossfuzz_fuzz_fn)(const uint8_t *data, size_t size,
                                 uint8_t *out, size_t *out_size);

/*
 * Filter target: receives input, writes transformed output to out/out_size,
 * sets *accepted to 1 (accept) or 0 (reject).
 * Return 0 on success, non-zero on error.
 */
typedef int (*crossfuzz_filter_fn)(const uint8_t *data, size_t size,
                                   uint8_t *out, size_t *out_size,
                                   int *accepted);

/*
 * Compare target: receives the fuzz input and arrays of target names/outputs.
 * Return NULL or "" if outputs match, or a string describing the mismatch.
 * The returned string must remain valid until the next call.
 */
typedef const char* (*crossfuzz_compare_fn)(const uint8_t *input, size_t input_size,
                                            int num_targets,
                                            const char **target_names,
                                            const uint8_t **target_outputs,
                                            const size_t *target_output_sizes);

/* ---- Entry points ---- */

/*
 * Start the fuzz harness loop with the given target and settings.
 * Pass NULL for settings to use defaults.
 */
int crossfuzz_fuzz(crossfuzz_fuzz_fn fn, const crossfuzz_settings_t *settings);

/*
 * Start the filter harness loop with the given target and settings.
 * Pass NULL for settings to use defaults.
 */
int crossfuzz_filter(crossfuzz_filter_fn fn, const crossfuzz_settings_t *settings);

/*
 * Start the comparator harness loop with the given target and settings.
 * Pass NULL for settings to use defaults.
 * The comparator reads CROSSFUZZ_SHM_TARGETS (JSON map of target name to
 * SHM path) and opens each target's shared memory to read outputs directly.
 */
int crossfuzz_compare(crossfuzz_compare_fn fn, const crossfuzz_settings_t *settings);

/* ---- Standalone functions ---- */

/*
 * Open and map the shared memory region from CROSSFUZZ_SHM.
 * Returns 0 on success, -1 on error.
 */
int crossfuzz_open_shm(void);

/*
 * Point the coverage bitmap at the shared memory region.
 * Must be called after crossfuzz_open_shm().
 * Returns 0 on success, -1 if SHM is not open.
 */
int crossfuzz_start_instrumentation(void);

/*
 * Clear the coverage bitmap (memset to 0).
 * No-op if SHM or instrumentation is not initialized.
 */
void crossfuzz_clear_instrumentation(void);

/*
 * Collect instrumentation data. For C (SanitizerCoverage) this is a no-op
 * since coverage writes directly to the bitmap.
 */
void crossfuzz_collect_instrumentation(void);

/*
 * Set the status field in the SHM header.
 */
void crossfuzz_set_status(uint32_t status);

#endif /* CROSSFUZZ_H */
