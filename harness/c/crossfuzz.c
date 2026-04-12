#include "crossfuzz.h"

#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <unistd.h>
#include <fcntl.h>
#include <sys/mman.h>
#include <sys/stat.h>

/* ---- Shared memory layout (must match Go pkg/coverage) ---- */

#define HEADER_SIZE        64
#define INPUT_OFFSET       64
#define INPUT_SIZE         (1 << 20)   /* 1 MB */
#define OUTPUT_OFFSET      (INPUT_OFFSET + INPUT_SIZE)
#define OUTPUT_SIZE        (1 << 20)   /* 1 MB */
#define COVERAGE_OFFSET    (OUTPUT_OFFSET + OUTPUT_SIZE)
#define COVERAGE_SIZE      (1 << 16)   /* 64 KB */
#define TOTAL_SHM_SIZE     (COVERAGE_OFFSET + COVERAGE_SIZE)

/* Header field offsets */
#define OFF_INPUT_LEN   8
#define OFF_OUTPUT_LEN  12
#define OFF_STATUS      16

/* Status codes */
#define STATUS_OK    0
#define STATUS_ERROR 1

/* Protocol pipe file descriptors (inherited from parent via ExtraFiles) */
#define CMD_FD  3
#define RESP_FD 4

/* Max number of targets for the compare harness */
#define MAX_COMPARE_TARGETS 64

/* ---- Globals ---- */

static uint8_t *shm_base;
static uint8_t *cov_bitmap;

/* ---- Low-level I/O ---- */

static int read_exact(int fd, void *buf, size_t n)
{
    size_t done = 0;
    while (done < n) {
        ssize_t r = read(fd, (uint8_t *)buf + done, n - done);
        if (r <= 0) return -1;
        done += (size_t)r;
    }
    return 0;
}

static int write_exact(int fd, const void *buf, size_t n)
{
    size_t done = 0;
    while (done < n) {
        ssize_t w = write(fd, (const uint8_t *)buf + done, n - done);
        if (w <= 0) return -1;
        done += (size_t)w;
    }
    return 0;
}

/* ---- Length-prefixed JSON protocol ---- */

static char *proto_read(int fd)
{
    uint8_t hdr[4];
    if (read_exact(fd, hdr, 4) < 0) return NULL;
    uint32_t len = ((uint32_t)hdr[0] << 24) | ((uint32_t)hdr[1] << 16) |
                   ((uint32_t)hdr[2] << 8)  | (uint32_t)hdr[3];
    if (len > (1 << 20)) return NULL;
    char *buf = (char *)malloc(len + 1);
    if (!buf) return NULL;
    if (read_exact(fd, buf, len) < 0) { free(buf); return NULL; }
    buf[len] = '\0';
    return buf;
}

static int proto_write(int fd, const char *json)
{
    uint32_t len = (uint32_t)strlen(json);
    uint8_t hdr[4] = {
        (uint8_t)(len >> 24), (uint8_t)(len >> 16),
        (uint8_t)(len >> 8),  (uint8_t)len
    };
    if (write_exact(fd, hdr, 4) < 0) return -1;
    if (write_exact(fd, json, len) < 0) return -1;
    return 0;
}

/* ---- Shared memory accessors ---- */

static uint32_t shm_get_u32(size_t off)
{
    uint32_t v;
    memcpy(&v, shm_base + off, 4);
    return v;
}

static void shm_set_u32(size_t off, uint32_t v)
{
    memcpy(shm_base + off, &v, 4);
}

/* ---- SanitizerCoverage callbacks ---- */

void __sanitizer_cov_trace_pc_guard_init(uint32_t *start, uint32_t *stop)
{
    if (start == stop || *start) return;
    for (uint32_t *p = start; p < stop; p++)
        *p = (uint32_t)(p - start) + 1;
}

void __sanitizer_cov_trace_pc_guard(uint32_t *guard)
{
    if (!cov_bitmap || !*guard) return;
    uint32_t idx = *guard % COVERAGE_SIZE;
    if (cov_bitmap[idx] < 255)
        cov_bitmap[idx]++;
}

/* ---- Simple JSON type detection ---- */

static int msg_is_type(const char *json, const char *type_val)
{
    char pat[128];
    snprintf(pat, sizeof(pat), "\"type\":\"%s\"", type_val);
    if (strstr(json, pat)) return 1;
    snprintf(pat, sizeof(pat), "\"type\": \"%s\"", type_val);
    if (strstr(json, pat)) return 1;
    return 0;
}

/* ---- Minimal JSON string array parser for "targets" field ---- */

/*
 * Parse the "targets" array from a JSON message.
 * Fills names[] with pointers into the msg buffer (modifies msg in-place
 * by null-terminating strings). Returns the number of targets found.
 */
static int parse_targets_array(char *msg, const char **names, int max_names)
{
    char *p = strstr(msg, "\"targets\"");
    if (!p) return 0;
    p = strchr(p, '[');
    if (!p) return 0;
    p++; /* skip '[' */

    int count = 0;
    while (*p && *p != ']' && count < max_names) {
        /* skip whitespace and commas */
        while (*p == ' ' || *p == ',' || *p == '\n' || *p == '\r' || *p == '\t') p++;
        if (*p == ']' || *p == '\0') break;
        if (*p != '"') { p++; continue; }
        p++; /* skip opening quote */
        names[count] = p;
        while (*p && *p != '"') p++;
        if (*p == '"') { *p = '\0'; p++; }
        count++;
    }
    return count;
}

/* ---- Minimal JSON map parser for CROSSFUZZ_SHM_TARGETS ---- */

typedef struct {
    char name[256];
    char path[1024];
    uint8_t *data;
} target_shm_entry_t;

/*
 * Parse {"key":"value",...} from env_str. Returns count of entries parsed.
 * Entries are written to entries[]. env_str is modified in-place.
 */
static int parse_shm_targets(char *env_str, target_shm_entry_t *entries, int max_entries)
{
    int count = 0;
    char *p = env_str;

    /* skip opening brace */
    while (*p && *p != '{') p++;
    if (*p == '{') p++;

    while (*p && *p != '}' && count < max_entries) {
        /* skip whitespace and commas */
        while (*p == ' ' || *p == ',' || *p == '\n' || *p == '\r' || *p == '\t') p++;
        if (*p == '}' || *p == '\0') break;

        /* parse key */
        if (*p != '"') { p++; continue; }
        p++;
        char *key_start = p;
        while (*p && *p != '"') p++;
        if (*p == '"') { *p = '\0'; p++; }

        /* skip colon */
        while (*p == ' ' || *p == ':') p++;

        /* parse value */
        if (*p != '"') { p++; continue; }
        p++;
        char *val_start = p;
        while (*p && *p != '"') p++;
        if (*p == '"') { *p = '\0'; p++; }

        snprintf(entries[count].name, sizeof(entries[count].name), "%s", key_start);
        snprintf(entries[count].path, sizeof(entries[count].path), "%s", val_start);
        entries[count].data = NULL;
        count++;
    }
    return count;
}

/* ---- Standalone functions ---- */

int crossfuzz_open_shm(void)
{
    const char *shm_path = getenv("CROSSFUZZ_SHM");
    if (!shm_path) {
        fprintf(stderr, "crossfuzz: CROSSFUZZ_SHM not set\n");
        return -1;
    }

    int fd = open(shm_path, O_RDWR);
    if (fd < 0) {
        perror("crossfuzz: open shm");
        return -1;
    }
    shm_base = (uint8_t *)mmap(NULL, TOTAL_SHM_SIZE,
                                PROT_READ | PROT_WRITE, MAP_SHARED, fd, 0);
    close(fd);
    if (shm_base == MAP_FAILED) {
        shm_base = NULL;
        perror("crossfuzz: mmap");
        return -1;
    }
    return 0;
}

int crossfuzz_start_instrumentation(void)
{
    if (!shm_base) return -1;
    cov_bitmap = shm_base + COVERAGE_OFFSET;
    return 0;
}

void crossfuzz_clear_instrumentation(void)
{
    if (cov_bitmap)
        memset(cov_bitmap, 0, COVERAGE_SIZE);
}

void crossfuzz_collect_instrumentation(void)
{
    /* No-op for C: SanitizerCoverage writes directly to the bitmap. */
}

void crossfuzz_set_status(uint32_t status)
{
    if (shm_base)
        shm_set_u32(OFF_STATUS, status);
}

/* ---- Helper: resolve settings with defaults ---- */

static crossfuzz_settings_t resolve_settings(const crossfuzz_settings_t *settings)
{
    if (settings) return *settings;
    return crossfuzz_default_settings();
}

/* ---- Fuzz entry point ---- */

int crossfuzz_fuzz(crossfuzz_fuzz_fn fn, const crossfuzz_settings_t *settings)
{
    crossfuzz_settings_t s = resolve_settings(settings);

    if (crossfuzz_open_shm() < 0) return 1;

    if (s.instrument) {
        crossfuzz_start_instrumentation();
    }

    if (proto_write(RESP_FD, "{\"type\":\"ready\"}") < 0) {
        fprintf(stderr, "crossfuzz: failed to send ready\n");
        return 1;
    }

    for (;;) {
        char *msg = proto_read(CMD_FD);
        if (!msg) break;

        if (msg_is_type(msg, "shutdown")) {
            free(msg);
            break;
        }

        if (msg_is_type(msg, "fuzz")) {
            free(msg);

            uint32_t in_len = shm_get_u32(OFF_INPUT_LEN);
            if (in_len > INPUT_SIZE) in_len = INPUT_SIZE;

            uint8_t *out_buf = shm_base + OUTPUT_OFFSET;
            size_t out_len = 0;

            int ret = fn(shm_base + INPUT_OFFSET, in_len, out_buf, &out_len);

            if (out_len > OUTPUT_SIZE) out_len = OUTPUT_SIZE;
            shm_set_u32(OFF_OUTPUT_LEN, (uint32_t)out_len);
            shm_set_u32(OFF_STATUS, ret == 0 ? STATUS_OK : STATUS_ERROR);

            if (ret == 0)
                proto_write(RESP_FD, "{\"type\":\"fuzz_result\",\"ok\":true}");
            else
                proto_write(RESP_FD,
                    "{\"type\":\"fuzz_result\",\"ok\":false,"
                    "\"error\":\"target returned error\"}");
        } else {
            free(msg);
        }
    }

    munmap(shm_base, TOTAL_SHM_SIZE);
    return 0;
}

/* ---- Filter entry point ---- */

int crossfuzz_filter(crossfuzz_filter_fn fn, const crossfuzz_settings_t *settings)
{
    crossfuzz_settings_t s = resolve_settings(settings);

    if (crossfuzz_open_shm() < 0) return 1;

    /* Filters typically don't need coverage instrumentation, but respect
     * the setting in case the user wants it. */
    if (s.instrument) {
        crossfuzz_start_instrumentation();
    }

    if (proto_write(RESP_FD, "{\"type\":\"ready\"}") < 0) {
        fprintf(stderr, "crossfuzz: failed to send ready\n");
        return 1;
    }

    for (;;) {
        char *msg = proto_read(CMD_FD);
        if (!msg) break;

        if (msg_is_type(msg, "shutdown")) {
            free(msg);
            break;
        }

        if (msg_is_type(msg, "filter")) {
            free(msg);

            uint32_t in_len = shm_get_u32(OFF_INPUT_LEN);
            if (in_len > INPUT_SIZE) in_len = INPUT_SIZE;

            uint8_t *out_buf = shm_base + OUTPUT_OFFSET;
            size_t out_len = 0;
            int accepted = 0;

            int ret = fn(shm_base + INPUT_OFFSET, in_len, out_buf, &out_len, &accepted);

            if (ret != 0) accepted = 0;

            if (accepted) {
                if (s.transform && out_len > 0) {
                    if (out_len > OUTPUT_SIZE) out_len = OUTPUT_SIZE;
                    shm_set_u32(OFF_OUTPUT_LEN, (uint32_t)out_len);
                } else {
                    /* Copy input to output region as-is. */
                    if (in_len > OUTPUT_SIZE) in_len = OUTPUT_SIZE;
                    memcpy(shm_base + OUTPUT_OFFSET, shm_base + INPUT_OFFSET, in_len);
                    shm_set_u32(OFF_OUTPUT_LEN, in_len);
                }
                proto_write(RESP_FD, "{\"type\":\"filter_result\",\"ok\":true}");
            } else {
                shm_set_u32(OFF_OUTPUT_LEN, 0);
                proto_write(RESP_FD, "{\"type\":\"filter_result\",\"ok\":false}");
            }
        } else {
            free(msg);
        }
    }

    munmap(shm_base, TOTAL_SHM_SIZE);
    return 0;
}

/* ---- Compare entry point ---- */

int crossfuzz_compare(crossfuzz_compare_fn fn, const crossfuzz_settings_t *settings)
{
    (void)settings; /* compare ignores settings currently */

    /* Parse CROSSFUZZ_SHM_TARGETS to get target SHM paths. */
    const char *targets_env = getenv("CROSSFUZZ_SHM_TARGETS");
    if (!targets_env) {
        fprintf(stderr, "crossfuzz: CROSSFUZZ_SHM_TARGETS not set\n");
        return 1;
    }

    /* Make a mutable copy for parsing. */
    size_t env_len = strlen(targets_env);
    char *env_copy = (char *)malloc(env_len + 1);
    if (!env_copy) return 1;
    memcpy(env_copy, targets_env, env_len + 1);

    target_shm_entry_t entries[MAX_COMPARE_TARGETS];
    int num_targets = parse_shm_targets(env_copy, entries, MAX_COMPARE_TARGETS);
    free(env_copy);

    if (num_targets == 0) {
        fprintf(stderr, "crossfuzz: no targets found in CROSSFUZZ_SHM_TARGETS\n");
        return 1;
    }

    /* mmap each target's SHM read-only. */
    for (int i = 0; i < num_targets; i++) {
        int fd = open(entries[i].path, O_RDONLY);
        if (fd < 0) {
            fprintf(stderr, "crossfuzz: open target SHM %s (%s): ", entries[i].name, entries[i].path);
            perror("");
            return 1;
        }
        entries[i].data = (uint8_t *)mmap(NULL, TOTAL_SHM_SIZE,
                                           PROT_READ, MAP_SHARED, fd, 0);
        close(fd);
        if (entries[i].data == MAP_FAILED) {
            fprintf(stderr, "crossfuzz: mmap target SHM %s: ", entries[i].name);
            perror("");
            return 1;
        }
    }

    if (proto_write(RESP_FD, "{\"type\":\"ready\"}") < 0) {
        fprintf(stderr, "crossfuzz: failed to send ready\n");
        return 1;
    }

    /* Reusable arrays for the callback. */
    const char *cb_names[MAX_COMPARE_TARGETS];
    const uint8_t *cb_outputs[MAX_COMPARE_TARGETS];
    size_t cb_output_sizes[MAX_COMPARE_TARGETS];

    for (;;) {
        char *msg = proto_read(CMD_FD);
        if (!msg) break;

        if (msg_is_type(msg, "shutdown")) {
            free(msg);
            break;
        }

        if (msg_is_type(msg, "compare")) {
            /* Parse requested target names from the message. */
            const char *req_names[MAX_COMPARE_TARGETS];
            int num_req = parse_targets_array(msg, req_names, MAX_COMPARE_TARGETS);

            /* Read input from the first matched target's SHM. */
            const uint8_t *input = NULL;
            size_t input_size = 0;
            int cb_count = 0;

            for (int r = 0; r < num_req; r++) {
                /* Find this target in our entries. */
                for (int e = 0; e < num_targets; e++) {
                    if (strcmp(req_names[r], entries[e].name) == 0) {
                        uint8_t *base = entries[e].data;

                        if (!input) {
                            uint32_t ilen;
                            memcpy(&ilen, base + OFF_INPUT_LEN, 4);
                            if (ilen > INPUT_SIZE) ilen = INPUT_SIZE;
                            input = base + INPUT_OFFSET;
                            input_size = ilen;
                        }

                        uint32_t olen;
                        memcpy(&olen, base + OFF_OUTPUT_LEN, 4);
                        if (olen > OUTPUT_SIZE) olen = OUTPUT_SIZE;

                        cb_names[cb_count] = entries[e].name;
                        cb_outputs[cb_count] = base + OUTPUT_OFFSET;
                        cb_output_sizes[cb_count] = olen;
                        cb_count++;
                        break;
                    }
                }
            }

            const char *mismatch = fn(input, input_size, cb_count,
                                      cb_names, cb_outputs, cb_output_sizes);

            free(msg);

            if (mismatch && mismatch[0] != '\0') {
                /* Build response with error. We need to escape the mismatch
                 * string for JSON, but for simplicity we just truncate at
                 * any quote character. */
                char resp[4096];
                char escaped[2048];
                size_t j = 0;
                for (size_t i = 0; mismatch[i] && j < sizeof(escaped) - 2; i++) {
                    if (mismatch[i] == '"') {
                        escaped[j++] = '\\';
                        escaped[j++] = '"';
                    } else if (mismatch[i] == '\\') {
                        escaped[j++] = '\\';
                        escaped[j++] = '\\';
                    } else if (mismatch[i] == '\n') {
                        escaped[j++] = '\\';
                        escaped[j++] = 'n';
                    } else {
                        escaped[j++] = mismatch[i];
                    }
                }
                escaped[j] = '\0';
                snprintf(resp, sizeof(resp),
                    "{\"type\":\"compare_result\",\"error\":\"%s\"}", escaped);
                proto_write(RESP_FD, resp);
            } else {
                proto_write(RESP_FD, "{\"type\":\"compare_result\"}");
            }
        } else {
            free(msg);
        }
    }

    /* Cleanup target mmaps. */
    for (int i = 0; i < num_targets; i++) {
        if (entries[i].data && entries[i].data != MAP_FAILED)
            munmap(entries[i].data, TOTAL_SHM_SIZE);
    }

    return 0;
}
