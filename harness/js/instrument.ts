/**
 * Bun preload plugin for Istanbul coverage instrumentation.
 *
 * Load this before the target to enable coverage-guided fuzzing:
 *   bun run --preload ../../harness/js/instrument.ts ./target.ts
 *
 * The plugin intercepts .ts module loads (excluding node_modules and the
 * harness itself), transpiles them to JS via Bun.Transpiler, then instruments
 * with istanbul-lib-instrument. The resulting __coverage__ global is read by
 * crossfuzz.ts after each target execution.
 *
 * NOTE: The filter is intentionally restricted to .tsx? (TypeScript only).
 * istanbul-lib-instrument uses @babel/core internally, which synchronously
 * require()s .js helper packages. If the plugin intercepted .js files too,
 * those require() calls would trigger our onLoad handler and Bun would throw
 * "onLoad() expects an object returned" because we'd return undefined for them.
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
    // Only match TypeScript source files. .js files (including all of
    // @babel/core's internal helpers) are left to Bun's default loader.
    build.onLoad({ filter: /\.tsx?$/ }, async ({ path }) => {
      const source = await Bun.file(path).text();

      // Transpile TypeScript → JavaScript before Istanbul sees it.
      const transpiler = new Bun.Transpiler({ loader: "ts" });
      const js = transpiler.transformSync(source);

      // Don't instrument the harness or any dependencies — only user target code.
      // We must still return an object (never undefined) for every matched path;
      // returning undefined causes Bun 1.3.8 to throw "onLoad() expects an object".
      if (path.includes("/node_modules/") || path.includes("/harness/js/")) {
        return { contents: js, loader: "js" };
      }

      const instrumented = instrumenter.instrumentSync(js, path);
      return { contents: instrumented, loader: "js" };
    });
  },
});
