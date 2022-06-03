package orchestrator

import (
	"context"
	"fmt"
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
	t.Skip("abourget: incomplete, untested")

	storeSplit := uint64(10)
	mods := manifest.NewTestModules()
	graph, err := manifest.NewModuleGraph(mods)
	require.NoError(t, err)

	storeMods, err := graph.StoresDownTo([]string{"G"})
	require.NoError(t, err)

	mockDStore := dstore.NewMockStore(nil)
	stores := map[string]*state.Store{}
	for _, mod := range storeMods {
		kindStore := mod.Kind.(*pbsubstreams.Module_KindStore_).KindStore
		newStore, err := state.NewBuilder(mod.Name, storeSplit, mod.InitialBlock, "myhash", kindStore.UpdatePolicy, kindStore.ValueType, mockDStore)
		require.NoError(t, err)
		stores[newStore.Name] = newStore
	}

	splitWorkMods := SplitWorkModules{
		"A": &SplitWork{modName: "A"},
		"B": &SplitWork{modName: "B"},
		"C": &SplitWork{modName: "C"},
		"D": &SplitWork{modName: "D"},
		"E": &SplitWork{modName: "E"},
		"F": &SplitWork{modName: "F"},
		"G": &SplitWork{modName: "G"},
		"H": &SplitWork{modName: "H"},
		"K": &SplitWork{modName: "K"},
	}

	pool := NewRequestPool()
	ctx := context.Background()
	s, err := NewOrderedStrategy(
		ctx,
		splitWorkMods,
		&pbsubstreams.Request{
			StartBlockNum: 0,
			StopBlockNum:  30,
		},
		stores, // INIT
		graph,
		pool,
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

	reqChan := s.getRequestStream(ctx)
	for req := range reqChan {
		fmt.Println(jobstr(req))
		allRequests = append(allRequests, jobstr(req))
	}

	fmt.Println(allRequests)

	assert.Equal(t, 8, len(allRequests))
}

func jobstr(j *Job) string {
	return fmt.Sprintf("%s %d-%d", j.moduleName, j.reqChunk.start, j.reqChunk.end)
}
