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
		expectError        bool
	}{
		{
			name:               "startblock too low",
			startBlock:         10,
			linearHandoffBlock: 20,
			exclusiveEndBlock:  80,
			moduleName:         "all_assert_init_20",
			expectError:        true,
		},
		{
			name:               "linear mode test",
			startBlock:         20,
			linearHandoffBlock: 20,
			exclusiveEndBlock:  80,
			moduleName:         "all_assert_init_20",
		},
		{
			name:               "starting before unaligned stores test",
			startBlock:         20,
			linearHandoffBlock: 100,
			exclusiveEndBlock:  120,
			moduleName:         "all_assert_init_20",
		},
		{
			name:               "starting after unaligned stores test",
			startBlock:         50,
			linearHandoffBlock: 100,
			exclusiveEndBlock:  120,
			moduleName:         "all_assert_init_20",
		},
		{
			name:               "set sum",
			startBlock:         0,
			linearHandoffBlock: 100,
			exclusiveEndBlock:  120,
			moduleName:         "assert_set_sum_store_0",
		},
		{
			name:               "set sum deltas",
			startBlock:         0,
			linearHandoffBlock: 100,
			exclusiveEndBlock:  120,
			moduleName:         "assert_set_sum_store_deltas_0",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			run := newTestRun(t, int64(c.startBlock), c.linearHandoffBlock, c.exclusiveEndBlock, 0, c.moduleName, "./testdata/complex_substreams/complex-substreams-v0.1.0.spkg")
			err := run.Run(t, c.moduleName)
			if c.expectError {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}

	//assert.Len(t, listFiles(t, run.TempDir), 90) // All these .kv files on disk
	// TODO: we don't produce those files when in linear mode..
	// because it produced inconsistent snapshots..
}
