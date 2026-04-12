#include "crossfuzz.hpp"

#include <cstring>

extern "C" {
#include "../c/crossfuzz.h"
}

namespace crossfuzz {

// ---- Internal trampoline state ----

namespace detail {
static FuzzFn g_fuzz_fn;
static FilterFn g_filter_fn;
static CompareFn g_compare_fn;
} // namespace detail

// ---- Settings conversion ----

static crossfuzz_settings_t to_c_settings(const Settings& s)
{
    crossfuzz_settings_t cs;
    cs.instrument = s.instrument ? 1 : 0;
    cs.warmup = s.warmup;
    cs.transform = s.transform ? 1 : 0;
    cs.hinting = s.hinting ? 1 : 0;
    return cs;
}

// ---- Fuzz ----

static int fuzz_trampoline(const uint8_t *data, size_t size,
                           uint8_t *out, size_t *out_size)
{
    try {
        auto result = detail::g_fuzz_fn(std::span<const uint8_t>(data, size));
        constexpr size_t kMaxOutput = 1u << 20;
        if (result.size() > kMaxOutput) result.resize(kMaxOutput);
        *out_size = result.size();
        std::memcpy(out, result.data(), result.size());
        return 0;
    } catch (...) {
        *out_size = 0;
        return 1;
    }
}

int fuzz(FuzzFn fn, Settings settings)
{
    detail::g_fuzz_fn = std::move(fn);
    auto cs = to_c_settings(settings);
    return crossfuzz_fuzz(fuzz_trampoline, &cs);
}

// ---- Filter ----

static int filter_trampoline(const uint8_t *data, size_t size,
                             uint8_t *out, size_t *out_size,
                             int *accepted)
{
    try {
        auto [result, acc] = detail::g_filter_fn(
            std::span<const uint8_t>(data, size));
        *accepted = acc ? 1 : 0;
        if (acc) {
            constexpr size_t kMaxOutput = 1u << 20;
            if (result.size() > kMaxOutput) result.resize(kMaxOutput);
            *out_size = result.size();
            std::memcpy(out, result.data(), result.size());
        } else {
            *out_size = 0;
        }
        return 0;
    } catch (...) {
        *out_size = 0;
        *accepted = 0;
        return 1;
    }
}

int filter(FilterFn fn, Settings settings)
{
    detail::g_filter_fn = std::move(fn);
    auto cs = to_c_settings(settings);
    return crossfuzz_filter(filter_trampoline, &cs);
}

// ---- Compare ----

static const char *compare_trampoline(const uint8_t *input, size_t input_size,
                                       int num_targets,
                                       const char **target_names,
                                       const uint8_t **target_outputs,
                                       const size_t *target_output_sizes)
{
    static thread_local std::string result_buf;
    try {
        std::vector<std::string> names;
        names.reserve(num_targets);
        std::vector<std::vector<uint8_t>> outputs;
        outputs.reserve(num_targets);
        for (int i = 0; i < num_targets; i++) {
            names.emplace_back(target_names[i]);
            outputs.emplace_back(target_outputs[i],
                                 target_outputs[i] + target_output_sizes[i]);
        }
        result_buf = detail::g_compare_fn(
            std::span<const uint8_t>(input, input_size), names, outputs);
        return result_buf.empty() ? nullptr : result_buf.c_str();
    } catch (...) {
        result_buf = "compare function threw exception";
        return result_buf.c_str();
    }
}

int compare(CompareFn fn, Settings settings)
{
    detail::g_compare_fn = std::move(fn);
    auto cs = to_c_settings(settings);
    return crossfuzz_compare(compare_trampoline, &cs);
}

// ---- Standalone functions ----

int openShm() { return crossfuzz_open_shm(); }
int startInstrumentation() { return crossfuzz_start_instrumentation(); }
void clearInstrumentation() { crossfuzz_clear_instrumentation(); }
void collectInstrumentation() { crossfuzz_collect_instrumentation(); }
void setStatus(uint32_t status) { crossfuzz_set_status(status); }

} // namespace crossfuzz
