package store

import (
	"context"
	"fmt"
	"io"
	"testing"

	"github.com/streamingfast/dstore"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConfig_ListSnapshotFiles(t *testing.T) {
	testStore := dstore.NewMockStore(func(base string, f io.Reader) (err error) {
		return nil
	})

	expectedFiles := []string{
		"0000001000-0000000000.kv",
		"0000002000-0000001000.kv",
		"0000003000-0000002000.kv",
		"0000004000-0000003000.kv",
		"0000004370-0000004000.partial",
	}

	errSent := false
	testStore.WalkFunc = func(ctx context.Context, prefix string, f func(filename string) error) error {
		for i := 0; i < len(expectedFiles); i++ {
			if i == 3 && !errSent {
				errSent = true
				return fmt.Errorf("random connection error")
			}

			if err := f(expectedFiles[i]); err != nil {
				return err
			}
		}
		return nil
	}

	c := &Config{objStore: testStore, segmentSize: 1000}

	files, err := c.ListSnapshotFiles(context.Background(), 0, 10000)
	require.NoError(t, err)

	var actualFiles []string
	for _, file := range files {
		actualFiles = append(actualFiles, file.Filename)
	}

	assert.Equal(t, expectedFiles, actualFiles)
}

func TestLowestAlignedBoundary(t *testing.T) {
	tests := []struct {
		name               string
		moduleInitialBlock uint64
		segmentSize        uint64
		expected           uint64
	}{
		{
			name:               "aligned initial block",
			moduleInitialBlock: 1000,
			segmentSize:        100,
			expected:           1000,
		},
		{
			name:               "unaligned initial block",
			moduleInitialBlock: 1234,
			segmentSize:        100,
			expected:           1300,
		},
		{
			name:               "initial block zero",
			moduleInitialBlock: 0,
			segmentSize:        100,
			expected:           0,
		},
		{
			name:               "large segment size",
			moduleInitialBlock: 5000,
			segmentSize:        10000,
			expected:           10000,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &Config{
				moduleInitialBlock: tt.moduleInitialBlock,
				segmentSize:        tt.segmentSize,
			}

			got := cfg.lowestAlignedBoundary()
			if got != tt.expected {
				t.Errorf("lowestAlignedBoundary() = %v, want %v", got, tt.expected)
			}
		})
	}
}
