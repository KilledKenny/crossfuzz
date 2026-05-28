// Package tests is a side-effect import target. It pulls in every test
// subpackage so each one's init() registers its tests with framework.
package tests

import (
	_ "github.com/KilledKenny/crossfuzz/e2e/tests/cli"
	_ "github.com/KilledKenny/crossfuzz/e2e/tests/comparers/byte_equal"
	_ "github.com/KilledKenny/crossfuzz/e2e/tests/comparers/custom"
	_ "github.com/KilledKenny/crossfuzz/e2e/tests/comparers/harness"
	_ "github.com/KilledKenny/crossfuzz/e2e/tests/comparers/json_structural"
	_ "github.com/KilledKenny/crossfuzz/e2e/tests/comparers/none"
	_ "github.com/KilledKenny/crossfuzz/e2e/tests/comparers/numeric"
	_ "github.com/KilledKenny/crossfuzz/e2e/tests/comparers/numeric_relative"
	_ "github.com/KilledKenny/crossfuzz/e2e/tests/coverage"
	_ "github.com/KilledKenny/crossfuzz/e2e/tests/differential"
	_ "github.com/KilledKenny/crossfuzz/e2e/tests/harness"
	_ "github.com/KilledKenny/crossfuzz/e2e/tests/input_filter"
	_ "github.com/KilledKenny/crossfuzz/e2e/tests/restart"
	_ "github.com/KilledKenny/crossfuzz/e2e/tests/subcommands"
)
