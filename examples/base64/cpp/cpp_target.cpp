#include "../../harness/cpp/crossfuzz.hpp"

#include <cstdint>
#include <span>
#include <string>
#include <vector>

static const char kAlphabet[] =
    "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/";

static std::vector<uint8_t> base64_encode(std::span<const uint8_t> input)
{
    const size_t in_len = input.size();
    const size_t out_len = 4 * ((in_len + 2) / 3);
    std::vector<uint8_t> out(out_len);

    size_t i = 0, j = 0;
    while (i < in_len) {
        uint32_t a = input[i++];
        uint32_t b = (i < in_len) ? input[i++] : 0;
        uint32_t c = (i < in_len) ? input[i++] : 0;
        uint32_t triple = (a << 16) | (b << 8) | c;

        out[j++] = static_cast<uint8_t>(kAlphabet[(triple >> 18) & 0x3F]);
        out[j++] = static_cast<uint8_t>(kAlphabet[(triple >> 12) & 0x3F]);
        out[j++] = static_cast<uint8_t>(kAlphabet[(triple >>  6) & 0x3F]);
        out[j++] = static_cast<uint8_t>(kAlphabet[ triple        & 0x3F]);
    }

    size_t mod = in_len % 3;
    if (mod > 0) {
        out[out_len - 1] = '=';
        if (mod == 1)
            out[out_len - 2] = '=';
    }

    return out;
}

int main()
{
    return crossfuzz::fuzz([](std::span<const uint8_t> input) {
        return base64_encode(input);
    });
}
