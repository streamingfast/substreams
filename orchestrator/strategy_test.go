package orchestrator

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/streamingfast/dstore"
	"github.com/streamingfast/substreams/manifest"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
	"github.com/streamingfast/substreams/state"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewOrderedStrategy_GetNextRequest(t *testing.T) {
	t.Skip("abourget: incomplete, untested")

	saveInterval := 10
	mods := manifest.NewTestModules()
	graph, err := manifest.NewModuleGraph(mods)
	require.NoError(t, err)

	storeMods, err := graph.StoresDownTo([]string{"G"})
	require.NoError(t, err)

	mockDStore := dstore.NewMockStore(nil)
	var stores []*state.Store
	for _, mod := range storeMods {
		kindStore := mod.Kind.(*pbsubstreams.Module_KindStore_).KindStore
		newStore, err := state.NewBuilder(mod.Name, uint64(saveInterval), mod.InitialBlock, "myhash", kindStore.UpdatePolicy, kindStore.ValueType, mockDStore)
		require.NoError(t, err)
		stores = append(stores, newStore)
	}

	pool := NewRequestPool()
	ctx := context.Background()
	storageState := &StorageState{lastBlocks: map[string]uint64{}}
	s, err := NewOrderedStrategy(
		ctx,
		storageState,
		&pbsubstreams.Request{
			StartBlockNum: 10_000,
			StopBlockNum:  100_000,
		},
		stores, // INIT
		graph,
		pool,
		100_000, // corresponds to `Request.EndBlock` doesn't it?
		saveInterval,
		1_000_000, // FIXME
	)
	require.NoError(t, err)

	var allreqs []string
	for {
		req, err := s.GetNextRequest(ctx)
		require.NoError(t, err)
		allreqs = append(allreqs, reqstr(req))
	}

	assert.Equal(t, []string{
		"insert",
		"the",
		"expected",
		"values",
	}, allreqs)
}

func reqstr(r *pbsubstreams.Request) string {
	return fmt.Sprintf("%s %d-%d", strings.Join(r.OutputModules, ","), r.StartBlockNum, r.StopBlockNum)
}
