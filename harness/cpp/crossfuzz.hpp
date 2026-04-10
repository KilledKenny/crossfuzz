#pragma once

#include <cstdint>
#include <functional>
#include <span>
#include <vector>

namespace crossfuzz {

using FuzzFn = std::function<std::vector<uint8_t>(std::span<const uint8_t>)>;

namespace detail {
// Trampoline state: one fuzz function registered per process.
extern FuzzFn g_fuzz_fn;
} // namespace detail

/*
 * Run the fuzzing harness loop. Call from main().
 *
 * The provided function receives the fuzz input and returns the output.
 * Throw any exception to signal an error for the current iteration;
 * the harness will report it as a non-zero status and continue.
 *
 * Internally delegates to the C harness crossfuzz_run(); compile with
 * both crossfuzz.c and crossfuzz.cpp from harness/cpp/.
 */
int run(FuzzFn fn);

} // namespace crossfuzz
