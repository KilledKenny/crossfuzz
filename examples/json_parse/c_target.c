#include "crossfuzz.h"
#include <string.h>
#include <stdint.h>

/*
 * Minimal recursive-descent JSON parser.
 * Returns the type of the root value as a string, or "error" if invalid.
 * This intentionally hand-rolls parsing so it can diverge from Go/Java.
 */

typedef struct {
    const uint8_t *src;
    size_t         len;
    size_t         pos;
} Parser;

static void skip_ws(Parser *p) {
    while (p->pos < p->len) {
        uint8_t c = p->src[p->pos];
        if (c == ' ' || c == '\t' || c == '\r' || c == '\n')
            p->pos++;
        else
            break;
    }
}

static int parse_value(Parser *p, uint8_t *out, size_t *out_len);

static int parse_string(Parser *p) {
    if (p->pos >= p->len || p->src[p->pos] != '"') return 0;
    p->pos++; /* skip opening quote */
    while (p->pos < p->len) {
        uint8_t c = p->src[p->pos++];
        if (c == '"') return 1;
        if (c == '\\') {
            if (p->pos >= p->len) return 0;
            uint8_t esc = p->src[p->pos++];
            if (esc == 'u') {
                /* \uXXXX: require exactly 4 hex digits */
                for (int i = 0; i < 4; i++) {
                    if (p->pos >= p->len) return 0;
                    uint8_t h = p->src[p->pos++];
                    if (!((h >= '0' && h <= '9') ||
                          (h >= 'a' && h <= 'f') ||
                          (h >= 'A' && h <= 'F')))
                        return 0;
                }
            }
            /* other escapes: accept anything after backslash */
        }
        /* control characters are invalid in JSON strings */
        if (c < 0x20 && c != '\\') return 0;
    }
    return 0; /* unterminated */
}

static int parse_number(Parser *p) {
    if (p->pos >= p->len) return 0;
    if (p->src[p->pos] == '-') p->pos++;
    if (p->pos >= p->len) return 0;
    if (p->src[p->pos] == '0') {
        p->pos++;
    } else {
        if (p->src[p->pos] < '1' || p->src[p->pos] > '9') return 0;
        while (p->pos < p->len && p->src[p->pos] >= '0' && p->src[p->pos] <= '9')
            p->pos++;
    }
    if (p->pos < p->len && p->src[p->pos] == '.') {
        p->pos++;
        if (p->pos >= p->len || p->src[p->pos] < '0' || p->src[p->pos] > '9') return 0;
        while (p->pos < p->len && p->src[p->pos] >= '0' && p->src[p->pos] <= '9')
            p->pos++;
    }
    if (p->pos < p->len && (p->src[p->pos] == 'e' || p->src[p->pos] == 'E')) {
        p->pos++;
        if (p->pos < p->len && (p->src[p->pos] == '+' || p->src[p->pos] == '-'))
            p->pos++;
        if (p->pos >= p->len || p->src[p->pos] < '0' || p->src[p->pos] > '9') return 0;
        while (p->pos < p->len && p->src[p->pos] >= '0' && p->src[p->pos] <= '9')
            p->pos++;
    }
    return 1;
}

static int parse_array(Parser *p) {
    if (p->pos >= p->len || p->src[p->pos] != '[') return 0;
    p->pos++;
    skip_ws(p);
    if (p->pos < p->len && p->src[p->pos] == ']') { p->pos++; return 1; }
    while (1) {
        uint8_t dummy[16]; size_t dl = 0;
        if (!parse_value(p, dummy, &dl)) return 0;
        skip_ws(p);
        if (p->pos >= p->len) return 0;
        if (p->src[p->pos] == ']') { p->pos++; return 1; }
        if (p->src[p->pos] != ',') return 0;
        p->pos++;
        skip_ws(p);
    }
}

static int parse_object(Parser *p) {
    if (p->pos >= p->len || p->src[p->pos] != '{') return 0;
    p->pos++;
    skip_ws(p);
    if (p->pos < p->len && p->src[p->pos] == '}') { p->pos++; return 1; }
    while (1) {
        skip_ws(p);
        if (!parse_string(p)) return 0;
        skip_ws(p);
        if (p->pos >= p->len || p->src[p->pos] != ':') return 0;
        p->pos++;
        skip_ws(p);
        uint8_t dummy[16]; size_t dl = 0;
        if (!parse_value(p, dummy, &dl)) return 0;
        skip_ws(p);
        if (p->pos >= p->len) return 0;
        if (p->src[p->pos] == '}') { p->pos++; return 1; }
        if (p->src[p->pos] != ',') return 0;
        p->pos++;
    }
}

static int match_literal(Parser *p, const char *lit) {
    size_t n = strlen(lit);
    if (p->pos + n > p->len) return 0;
    if (memcmp(p->src + p->pos, lit, n) != 0) return 0;
    p->pos += n;
    return 1;
}

static int parse_value(Parser *p, uint8_t *out, size_t *out_len) {
    skip_ws(p);
    if (p->pos >= p->len) return 0;
    uint8_t c = p->src[p->pos];
    const char *type = NULL;
    int ok = 0;

    if (c == '"')       { ok = parse_string(p); type = "string"; }
    else if (c == '{')  { ok = parse_object(p); type = "object"; }
    else if (c == '[')  { ok = parse_array(p);  type = "array";  }
    else if (c == 't')  { ok = match_literal(p, "true");  type = "true";  }
    else if (c == 'f')  { ok = match_literal(p, "false"); type = "false"; }
    else if (c == 'n')  { ok = match_literal(p, "null");  type = "null";  }
    else if (c == '-' || (c >= '0' && c <= '9')) { ok = parse_number(p); type = "number"; }

    if (!ok || type == NULL) return 0;
    *out_len = strlen(type);
    memcpy(out, type, *out_len);
    return 1;
}

int crossfuzz_target(const uint8_t *data, size_t size,
                     uint8_t *out, size_t *out_size)
{
    Parser p = { data, size, 0 };
    uint8_t type_buf[16];
    size_t  type_len = 0;

    if (!parse_value(&p, type_buf, &type_len)) {
        const char *err = "error";
        *out_size = strlen(err);
        memcpy(out, err, *out_size);
        return 0;
    }

    /* Reject trailing non-whitespace */
    skip_ws(&p);
    if (p.pos != p.len) {
        const char *err = "error";
        *out_size = strlen(err);
        memcpy(out, err, *out_size);
        return 0;
    }

    memcpy(out, type_buf, type_len);
    *out_size = type_len;
    return 0;
}

int main(void) {
    return crossfuzz_run();
}
