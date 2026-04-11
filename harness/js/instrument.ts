/**
 * Bun preload plugin for Istanbul coverage instrumentation.
 *
 * Load this before the target to enable coverage-guided fuzzing:
 *   bun run --preload ../../harness/js/instrument.ts ./target.ts
 *
 * The plugin intercepts .ts and .js module loads (excluding node_modules and
 * the harness itself), transpiles .ts to JS via Bun.Transpiler, then instruments
 * with istanbul-lib-instrument. The resulting __coverage__ global is read by
 * crossfuzz.ts after each target execution.
 *
 * NOTE: We must never return undefined from a matched onLoad handler — Bun 1.3.8
 * throws "onLoad() expects an object returned" in that case. For node_modules and
 * harness files we return the source unchanged so Bun still gets a valid object.
 */

import { plugin } from "bun";
import { createInstrumenter } from "istanbul-lib-instrument";

const instrumenter = createInstrumenter({
  esModules: true,
  compact: false,
  produceSourceMap: false,
  coverageVariable: "__coverage__",
});

plugin({
  name: "crossfuzz-istanbul",
  setup(build) {
    // Exclude node_modules via a negative lookahead so Bun never calls this
    // handler for @babel/core's CJS helpers. If it did, we'd return them with
    // loader:"js" (ESM), and @babel/core's synchronous require() of those
    // modules would fail with "require() async module is unsupported".
    build.onLoad({ filter: /^(?!.*\/node_modules\/).*\.[jt]sx?$/ }, async ({ path }) => {
      const source = await Bun.file(path).text();

      // Transpile .ts/.tsx → JavaScript; leave .js/.jsx as-is.
      let js: string;
      if (path.endsWith(".ts") || path.endsWith(".tsx")) {
        const transpiler = new Bun.Transpiler({ loader: "ts" });
        js = transpiler.transformSync(source);
      } else {
        js = source;
      }

      // Don't instrument the harness itself — only user target code.
      if (path.includes("/harness/js/")) {
        return { contents: js, loader: "js" };
      }

      const instrumented = instrumenter.instrumentSync(js, path);
      return { contents: instrumented, loader: "js" };
    });
  },
});
