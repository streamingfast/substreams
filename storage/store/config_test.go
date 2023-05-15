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

	c := &Config{objStore: testStore}

	files, err := c.ListSnapshotFiles(context.Background(), 10000)
	require.NoError(t, err)

	var actualFiles []string
	for _, file := range files {
		actualFiles = append(actualFiles, file.Filename)
	}

	assert.Equal(t, expectedFiles, actualFiles)
}
