// Package tests is a side-effect import target. It pulls in every test
// subpackage so each one's init() registers its tests with framework.
package tests

import (
	_ "crossfuzz/e2e/tests/cli"
	_ "crossfuzz/e2e/tests/comparers/byte_equal"
	_ "crossfuzz/e2e/tests/comparers/custom"
	_ "crossfuzz/e2e/tests/comparers/harness"
	_ "crossfuzz/e2e/tests/comparers/json_structural"
	_ "crossfuzz/e2e/tests/comparers/none"
	_ "crossfuzz/e2e/tests/comparers/numeric"
	_ "crossfuzz/e2e/tests/comparers/numeric_relative"
	_ "crossfuzz/e2e/tests/coverage"
	_ "crossfuzz/e2e/tests/differential"
	_ "crossfuzz/e2e/tests/harness"
	_ "crossfuzz/e2e/tests/input_filter"
	_ "crossfuzz/e2e/tests/restart"
	_ "crossfuzz/e2e/tests/subcommands"
)
