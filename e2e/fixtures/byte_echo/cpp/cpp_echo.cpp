#include "crossfuzz.hpp"

#include <cstdint>
#include <span>
#include <vector>

static std::vector<uint8_t> echo(std::span<const uint8_t> input)
{
    std::vector<uint8_t> out(input.size());
    for (size_t i = 0; i < input.size(); i++) {
        uint8_t b = input[i];
        if (b < 0x20)      out[i] = b;
        else if (b < 0x40) out[i] = b;
        else if (b < 0x60) out[i] = b;
        else if (b < 0x80) out[i] = b;
        else               out[i] = b;
    }
    return out;
}

int main()
{
    return crossfuzz::fuzz([](std::span<const uint8_t> in) { return echo(in); });
}
