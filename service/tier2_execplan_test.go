package service

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/streamingfast/dstore"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
	"github.com/streamingfast/substreams/pipeline/exec"
	"github.com/streamingfast/substreams/storage/execout"
	"github.com/streamingfast/substreams/storage/index"
	"github.com/streamingfast/substreams/storage/store"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// TestGetExecutionPlan_CancelledContextNoMapRace exercises the cleanup
// goroutine of GetExecutionPlan, which closes the already-opened exec output
// readers when the context is cancelled. That goroutine iterates over the
// existingExecOuts map while the main loop is still populating it: without
// synchronization this is a concurrent map iteration/write (caught by -race,
// and a fatal "concurrent map iteration and map write" in production).
func TestGetExecutionPlan_CancelledContextNoMapRace(t *testing.T) {
	logger := zap.NewNop()

	const moduleCount = 20
	modules := make([]*pbsubstreams.Module, moduleCount)
	for i := 0; i < moduleCount; i++ {
		mod := &pbsubstreams.Module{
			Name: fmt.Sprintf("mod_%d", i),
			Kind: &pbsubstreams.Module_KindMap_{},
		}
		if i == 0 {
			mod.Inputs = []*pbsubstreams.Module_Input{
				{
					Input: &pbsubstreams.Module_Input_Source_{
						Source: &pbsubstreams.Module_Input_Source{Type: "test.Block"},
					},
				},
			}
		} else {
			mod.Inputs = []*pbsubstreams.Module_Input{
				{
					Input: &pbsubstreams.Module_Input_Map_{
						Map: &pbsubstreams.Module_Input_Map{ModuleName: fmt.Sprintf("mod_%d", i-1)},
					},
				},
			}
		}
		modules[i] = mod
	}

	outputModule := fmt.Sprintf("mod_%d", moduleCount-1)
	execGraph, err := exec.NewOutputModuleGraph(outputModule, true, &pbsubstreams.Modules{
		Modules: modules,
		Binaries: []*pbsubstreams.Binary{
			{Type: "test", Content: []byte("some-fake-binary-data")},
		},
	}, 0)
	require.NoError(t, err)

	const stage = uint32(0)
	const startBlock = uint64(0)
	const stopBlock = uint64(10)

	// Pre-write an (empty, thus valid and "ordered") exec output file for
	// every module so that the main loop of GetExecutionPlan writes each of
	// them into the existingExecOuts map.
	baseStore := dstore.NewMockStore(nil)
	for _, mod := range execGraph.UsedModulesUpToStage(int(stage)) {
		hash := execGraph.ModuleHashes()[mod.Name]
		baseStore.SetFile(fmt.Sprintf("%s/outputs/%010d-%010d.output", hash, startBlock, stopBlock), []byte{})
	}

	execoutConfigs, err := execout.NewConfigs(baseStore, execGraph.UsedModulesUpToStage(int(stage)), execGraph.ModuleHashes(), stopBlock, 0, logger)
	require.NoError(t, err)

	storeConfigs, err := store.NewConfigMap(baseStore, nil, execGraph.Stores(), execGraph.ModuleHashes(), 0, 0, t.TempDir(), "memory")
	require.NoError(t, err)

	indexConfigs, err := index.NewConfigs(baseStore, execGraph.UsedIndexesModulesUpToStage(int(stage)), execGraph.ModuleHashes(), 0, logger)
	require.NoError(t, err)

	// The context is already cancelled when GetExecutionPlan starts: the
	// cleanup goroutine fires immediately and iterates the map concurrently
	// with the population loop.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	plan, err := GetExecutionPlan(ctx, logger, execGraph, stage, startBlock, stopBlock, outputModule, execoutConfigs, indexConfigs, storeConfigs)
	require.NoError(t, err)
	require.NotNil(t, plan)
	require.Len(t, plan.ExistingExecOuts, moduleCount)

	// Give the cleanup goroutine time to run its iteration before the test
	// ends, so the race detector observes both sides.
	time.Sleep(100 * time.Millisecond)
}

// TestGetExecutionPlan_StoreOutputMissingWithFullKV checks that a store whose fullKV
// exists at the segment end but whose exec output file is gone is re-executed only to
// replay its per-block state for the modules above it: its snapshot is never rewritten.
func TestGetExecutionPlan_StoreOutputMissingWithFullKV(t *testing.T) {
	logger := zap.NewNop()

	modules := []*pbsubstreams.Module{
		{
			Name: "store_a",
			Kind: &pbsubstreams.Module_KindStore_{KindStore: &pbsubstreams.Module_KindStore{
				UpdatePolicy: pbsubstreams.Module_KindStore_UPDATE_POLICY_SET,
				ValueType:    "string",
			}},
			Inputs: []*pbsubstreams.Module_Input{
				{Input: &pbsubstreams.Module_Input_Source_{Source: &pbsubstreams.Module_Input_Source{Type: "test.Block"}}},
			},
		},
		{
			Name: "map_a",
			Kind: &pbsubstreams.Module_KindMap_{},
			Inputs: []*pbsubstreams.Module_Input{
				{Input: &pbsubstreams.Module_Input_Source_{Source: &pbsubstreams.Module_Input_Source{Type: "test.Block"}}},
				{Input: &pbsubstreams.Module_Input_Store_{Store: &pbsubstreams.Module_Input_Store{
					ModuleName: "store_a",
					Mode:       pbsubstreams.Module_Input_Store_GET,
				}}},
			},
		},
	}

	execGraph, err := exec.NewOutputModuleGraph("map_a", true, &pbsubstreams.Modules{
		Modules: modules,
		Binaries: []*pbsubstreams.Binary{
			{Type: "test", Content: []byte("some-fake-binary-data")},
		},
	}, 0)
	require.NoError(t, err)

	const stage = uint32(1)
	const startBlock = uint64(100)
	const stopBlock = uint64(200)
	storeHash := execGraph.ModuleHashes()["store_a"]

	setup := func(t *testing.T, files ...string) *ExecutionPlan {
		baseStore := dstore.NewMockStore(nil)
		for _, file := range files {
			baseStore.SetFile(file, []byte{})
		}
		// The mock returns io.EOF for a missing file; sub-stores inherit this func with
		// the sub-folder stripped from `name`, hence the suffix match.
		baseStore.OpenObjectFunc = func(_ context.Context, name string) (io.ReadCloser, error) {
			for path, content := range baseStore.Files {
				if strings.HasSuffix(path, name) {
					return io.NopCloser(bytes.NewReader(content)), nil
				}
			}
			return nil, dstore.ErrNotFound
		}
		execoutConfigs, err := execout.NewConfigs(baseStore, execGraph.UsedModulesUpToStage(int(stage)), execGraph.ModuleHashes(), stopBlock-startBlock, 0, logger)
		require.NoError(t, err)
		storeConfigs, err := store.NewConfigMap(baseStore, nil, execGraph.Stores(), execGraph.ModuleHashes(), 0, 0, t.TempDir(), "memory")
		require.NoError(t, err)
		indexConfigs, err := index.NewConfigs(baseStore, execGraph.UsedIndexesModulesUpToStage(int(stage)), execGraph.ModuleHashes(), 0, logger)
		require.NoError(t, err)

		plan, err := GetExecutionPlan(context.Background(), logger, execGraph, stage, startBlock, stopBlock, "map_a", execoutConfigs, indexConfigs, storeConfigs)
		require.NoError(t, err)
		require.NotNil(t, plan)
		return plan
	}

	fullKV := fmt.Sprintf("%s/states/%010d-%010d.kv", storeHash, stopBlock, 0)
	output := fmt.Sprintf("%s/outputs/%010d-%010d.output", storeHash, startBlock, stopBlock)

	t.Run("output missing", func(t *testing.T) {
		plan := setup(t, fullKV)
		require.Contains(t, plan.RequiredModules, "store_a", "the store replays through wasm when its ops file is gone")
		require.NotContains(t, plan.StoresToWrite, "store_a", "an existing fullKV is never rebuilt")
		require.Contains(t, plan.ExecoutWriters, "store_a")
		require.False(t, plan.Skippable)
	})

	t.Run("output present", func(t *testing.T) {
		plan := setup(t, fullKV, output)
		require.NotContains(t, plan.RequiredModules, "store_a")
		require.NotContains(t, plan.StoresToWrite, "store_a")
		require.Contains(t, plan.ExistingExecOuts, "store_a")
	})
}
