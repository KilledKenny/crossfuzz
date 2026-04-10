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
    /* Match "type":"val" or "type": "val" */
    char pat[128];
    snprintf(pat, sizeof(pat), "\"type\":\"%s\"", type_val);
    if (strstr(json, pat)) return 1;
    snprintf(pat, sizeof(pat), "\"type\": \"%s\"", type_val);
    if (strstr(json, pat)) return 1;
    return 0;
}

/* ---- Harness entry point ---- */

int crossfuzz_run(void)
{
    const char *shm_path = getenv("CROSSFUZZ_SHM");
    if (!shm_path) {
        fprintf(stderr, "crossfuzz: CROSSFUZZ_SHM not set\n");
        return 1;
    }

    int fd = open(shm_path, O_RDWR);
    if (fd < 0) {
        perror("crossfuzz: open shm");
        return 1;
    }
    shm_base = (uint8_t *)mmap(NULL, TOTAL_SHM_SIZE,
                                PROT_READ | PROT_WRITE, MAP_SHARED, fd, 0);
    close(fd);
    if (shm_base == MAP_FAILED) {
        perror("crossfuzz: mmap");
        return 1;
    }
    cov_bitmap = shm_base + COVERAGE_OFFSET;

    /* Handshake: tell coordinator we're ready. */
    if (proto_write(RESP_FD, "{\"type\":\"ready\"}") < 0) {
        fprintf(stderr, "crossfuzz: failed to send ready\n");
        return 1;
    }

    /* Persistent-mode loop. */
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

            int ret = crossfuzz_target(shm_base + INPUT_OFFSET, in_len,
                                       out_buf, &out_len);

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
