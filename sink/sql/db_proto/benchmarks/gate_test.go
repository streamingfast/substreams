package benchmarks

import (
	"context"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/testcontainers/testcontainers-go"
)

// This package holds two different kinds of test, gated separately.
//
// A correctness test asserts something and should run wherever it can, which means
// anywhere a container can be started. A measurement asserts nothing — it prints a table
// for a human — and costs a minute or two, so it stays opt-in even on a machine that
// could run it.

var (
	dockerProbeOnce sync.Once
	dockerProbeErr  error
)

// requireDocker skips a test that needs a database container, but only when no container
// runtime is actually reachable. `go test ./...` has to work on machines without Docker,
// including the macOS CI runner, yet a test that is cheap once a daemon is there should
// not need an environment variable to run.
func requireDocker(t *testing.T) {
	t.Helper()

	dockerProbeOnce.Do(func() {
		provider, err := testcontainers.ProviderDefault.GetProvider()
		if err != nil {
			dockerProbeErr = err
			return
		}
		defer provider.Close()

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		dockerProbeErr = provider.Health(ctx)
	})

	if dockerProbeErr != nil {
		// The one place where the switch still matters: on a runner that is supposed to
		// have Docker, a probe failure is a broken runner, not a reason to pass silently.
		if os.Getenv("SF_SINK_SQL_INTEGRATION_TESTS") != "" {
			t.Fatalf("SF_SINK_SQL_INTEGRATION_TESTS is set but no container runtime is reachable: %s", dockerProbeErr)
		}

		t.Skipf("no container runtime reachable: %s", dockerProbeErr)
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
