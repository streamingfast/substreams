package benchmarks

import (
	"os"
	"testing"
)

// This package holds two different kinds of test, gated separately.
//
// A correctness test asserts something and should run wherever it can, which means
// anywhere the sink/sql integration tests run. A measurement asserts nothing — it prints
// a table for a human — and costs a minute or two, so it stays opt-in even on a machine
// that could run it.

// requireDocker skips a test that needs a database container. `go test ./...` has to work
// on machines without Docker, including the macOS CI runner, so these ride the same
// switch as the rest of the sink/sql integration tests.
func requireDocker(t *testing.T) {
	t.Helper()

	if os.Getenv("SF_SINK_SQL_INTEGRATION_TESTS") == "" {
		t.Skip("needs docker; set SF_SINK_SQL_INTEGRATION_TESTS=true to run")
	}
}

// requireBenchmark skips a measurement. These assert nothing, so running them in CI
// spends minutes producing a table nobody reads.
func requireBenchmark(t *testing.T) {
	t.Helper()

	if os.Getenv("SF_SINK_SQL_BENCHMARKS") == "" {
		t.Skip("measurement only; set SF_SINK_SQL_BENCHMARKS=true to run")
	}
}
