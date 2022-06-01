package orchestrator

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/streamingfast/dstore"
	"github.com/streamingfast/substreams/manifest"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
	"github.com/streamingfast/substreams/state"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewOrderedStrategy_GetNextRequest(t *testing.T) {
	//t.Skip("abourget: incomplete, untested")

	saveInterval := 10
	mods := manifest.NewTestModules()
	graph, err := manifest.NewModuleGraph(mods)
	require.NoError(t, err)

	storeMods, err := graph.StoresDownTo([]string{"G"})
	require.NoError(t, err)

	mockDStore := dstore.NewMockStore(nil)
	stores := map[string]*state.Store{}
	for _, mod := range storeMods {
		kindStore := mod.Kind.(*pbsubstreams.Module_KindStore_).KindStore
		newStore, err := state.NewBuilder(mod.Name, uint64(saveInterval), mod.InitialBlock, "myhash", kindStore.UpdatePolicy, kindStore.ValueType, mockDStore)
		require.NoError(t, err)
		stores[newStore.Name] = newStore
	}

	pool := NewRequestPool()
	ctx := context.Background()
	storageState := NewStorageState() //&StorageState{lastBlocks: map[string]uint64{}}
	s, err := NewOrderedStrategy(
		ctx,
		storageState,
		&pbsubstreams.Request{
			StartBlockNum: 0,
			StopBlockNum:  30,
		},
		stores, // INIT
		graph,
		pool,
		30, // corresponds to `Request.EndBlock` doesn't it?
		saveInterval,
		1_000_000, // FIXME
	)
	require.NoError(t, err)

	//simulate squasher squashing the data and notifying the pool
	go func() {
		time.Sleep(1 * time.Second)
		pool.Notify("E", 10)

		time.Sleep(1 * time.Second)
		pool.Notify("E", 20)
		time.Sleep(1 * time.Second)
		pool.Notify("B", 20)
	}()

	var allRequests []string

	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	reqChan := GetRequestStream(ctx, s)
	for req := range reqChan {
		fmt.Println(reqstr(req))
		allRequests = append(allRequests, reqstr(req))
	}

	fmt.Println(allRequests)

	assert.Equal(t, 8, len(allRequests))
}

func reqstr(r *pbsubstreams.Request) string {
	return fmt.Sprintf("%s %d-%d", strings.Join(r.OutputModules, ","), r.StartBlockNum, r.StopBlockNum)
}
