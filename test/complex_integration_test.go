package integration

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAllAssertionsInComplex(t *testing.T) {
	// Relies on `assert_all_test` having modInit == 1, so
	run := newTestRun(t, 30, 100, 120, "assert_test_third_store", "./testdata/complex_substreams/complex-substreams-v0.1.0.spkg")

	require.NoError(t, run.Run(t, "assert_test_third_store"))

	//assert.Len(t, listFiles(t, run.TempDir), 90) // All these .kv files on disk
	// TODO: we don't produce those files when in linear mode..
	// because it produced inconsistent snapshots..
}
