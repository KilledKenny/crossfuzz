#pragma once

#include <cstdint>
#include <functional>
#include <map>
#include <span>
#include <string>
#include <utility>
#include <vector>

namespace crossfuzz {

struct Settings {
    bool instrument = true;
    int warmup = 0;
    bool transform = false;
    bool hinting = false;
};

using FuzzFn = std::function<std::vector<uint8_t>(std::span<const uint8_t>)>;

using FilterFn = std::function<std::pair<std::vector<uint8_t>, bool>(
    std::span<const uint8_t>)>;

using CompareFn = std::function<std::string(
    std::span<const uint8_t> input,
    const std::vector<std::string>& target_names,
    const std::vector<std::vector<uint8_t>>& target_outputs)>;

int fuzz(FuzzFn fn, Settings settings = {});
int filter(FilterFn fn, Settings settings = {});
int compare(CompareFn fn, Settings settings = {});

// Standalone functions
int openShm();
int startInstrumentation();
void clearInstrumentation();
void collectInstrumentation();
void setStatus(uint32_t status);

} // namespace crossfuzz
