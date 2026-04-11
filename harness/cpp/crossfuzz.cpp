#include "crossfuzz.hpp"

#include <cstring>

extern "C" {
#include "../c/crossfuzz.h"
}

namespace crossfuzz {
namespace detail {
FuzzFn g_fuzz_fn;
} // namespace detail

int run(FuzzFn fn, Settings settings)
{
    detail::g_fuzz_fn = std::move(fn);
    crossfuzz_settings_t c_settings{};
    c_settings.disable_instrumentation = settings.disable_instrumentation ? 1 : 0;
    return crossfuzz_run_ex(&c_settings);
}

} // namespace crossfuzz

/*
 * C trampoline called by the C harness on every fuzz iteration.
 * Forwards to the registered C++ lambda and copies the result into
 * the shared memory output buffer.
 */
extern "C" int crossfuzz_target(const uint8_t *data, size_t size,
                                 uint8_t *out, size_t *out_size)
{
    try {
        auto result = crossfuzz::detail::g_fuzz_fn(
            std::span<const uint8_t>(data, size));
        constexpr size_t kMaxOutput = 1u << 20; // 1 MB
        if (result.size() > kMaxOutput)
            result.resize(kMaxOutput);
        *out_size = result.size();
        std::memcpy(out, result.data(), result.size());
        return 0;
    } catch (...) {
        *out_size = 0;
        return 1;
    }
}
