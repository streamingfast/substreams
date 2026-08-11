package benchmarks

import (
	"os"
	"testing"
)

// This package holds two different kinds of test.
//
// A correctness test asserts something and always runs, container and all: the suite
// needs a container runtime anyway, so a test that quietly skipped itself without one
// would only hide a broken environment.
//
// A measurement asserts nothing — it prints a table for a human — and costs a minute or
// two, so it stays opt-in even on a machine that could run it.

// requireBenchmark skips a measurement. These assert nothing, so running them in CI
// spends minutes producing a table nobody reads.
func requireBenchmark(t *testing.T) {
	t.Helper()

	if os.Getenv("SF_SINK_SQL_BENCHMARKS") == "" {
		t.Skip("measurement only; set SF_SINK_SQL_BENCHMARKS=true to run")
	}
}
