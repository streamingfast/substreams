package orchestrator

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"testing"

	"github.com/streamingfast/dstore"
	"github.com/streamingfast/substreams/block"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
	"github.com/streamingfast/substreams/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSquash(t *testing.T) {
	t.Skip("julien: stalls")
	writeCount := 0

	mockStore := dstore.NewMockStore(nil)
	mockStore.WriteObjectFunc = func(ctx context.Context, base string, f io.Reader) error {
		writeCount++
		return nil
	}

	mockStore.OpenObjectFunc = func(ctx context.Context, name string) (out io.ReadCloser, err error) {
		if name == "0000020000-0000010000.kv" || name == "0000030000-0000020000.partial" {
			return io.NopCloser(bytes.NewReader([]byte("{}"))), nil
		}
		return nil, fmt.Errorf("file %q not mocked", name)
	}

	planner := &JobsPlanner{AvailableJobs: make(chan *Job, 100)}

	s := store.NewTestKVStore(t, pbsubstreams.Module_KindStore_UPDATE_POLICY_SET, store.OutputValueTypeString, mockStore)
	squashable := NewStoreSquasher(s, 80_000, 10_000, 10, planner)
	go squashable.launch(context.Background())

	require.NoError(t, squashable.squash([]*block.Range{{20_000, 30_000}}))
	require.Equal(t, 0, writeCount)

	require.NoError(t, squashable.squash([]*block.Range{{70_000, 80_000}}))
	require.Equal(t, 0, writeCount)

	require.NoError(t, squashable.squash([]*block.Range{{10_000, 20_000}}))

	squashable.Shutdown(nil)
	require.Equal(t, 2, writeCount) //both [10_000,20_000) and [20_000,30_000) will be merged and written
	assert.True(t, planner.completed)
}
