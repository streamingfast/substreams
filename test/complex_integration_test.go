package integration

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAllAssertionsInComplex(t *testing.T) {
	// Relies on `assert_all_test` having modInit == 1, so
	run := newTestRun(t, 20, 100, 120, "all_test_assert", "./testdata/complex_substreams/complex-substreams-v0.1.0.spkg")

	require.NoError(t, run.Run(t, "all_test_assert"))

	//assert.Len(t, listFiles(t, run.TempDir), 90) // All these .kv files on disk
	// TODO: we don't produce those files when in linear mode..
	// because it produced inconsistent snapshots..
}
