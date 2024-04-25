package integration

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAllAssertionsInComplex(t *testing.T) {
	cases := []struct {
		name               string
		startBlock         uint64
		linearHandoffBlock uint64
		exclusiveEndBlock  uint64
		moduleName         string
	}{
		{
			name:               "sunny path",
			startBlock:         20,
			linearHandoffBlock: 100,
			exclusiveEndBlock:  120,
			moduleName:         "all_assert_init_20",
		},

		{
			name:               "failing test",
			startBlock:         50,
			linearHandoffBlock: 100,
			exclusiveEndBlock:  120,
			moduleName:         "all_test_assert_init_20",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			run := newTestRun(t, int64(c.startBlock), c.linearHandoffBlock, c.exclusiveEndBlock, c.moduleName, "./testdata/complex_substreams/complex-substreams-v0.1.0.spkg")
			require.NoError(t, run.Run(t, c.moduleName))
		})
	}

	//assert.Len(t, listFiles(t, run.TempDir), 90) // All these .kv files on disk
	// TODO: we don't produce those files when in linear mode..
	// because it produced inconsistent snapshots..
}
