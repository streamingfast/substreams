package state

import (
	"context"
	"testing"
	"time"

	"github.com/streamingfast/dstore"
	"github.com/streamingfast/substreams/manifest"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
	"github.com/stretchr/testify/assert"
	"github.com/test-go/testify/require"
)

var testModules = []*pbsubstreams.Module{
	{
		Name:   "A",
		Kind:   &pbsubstreams.Module_KindMap_{KindMap: &pbsubstreams.Module_KindMap{}},
		Inputs: nil,
	},
	{
		Name: "B",
		Kind: &pbsubstreams.Module_KindStore_{KindStore: &pbsubstreams.Module_KindStore{}},
		Inputs: []*pbsubstreams.Module_Input{
			{
				Input: &pbsubstreams.Module_Input_Store_{Store: &pbsubstreams.Module_Input_Store{
					ModuleName: "A",
				}},
			},
		},
	},
	{
		Name: "C",
		Kind: &pbsubstreams.Module_KindStore_{KindStore: &pbsubstreams.Module_KindStore{}},
		Inputs: []*pbsubstreams.Module_Input{
			{
				Input: &pbsubstreams.Module_Input_Store_{Store: &pbsubstreams.Module_Input_Store{
					ModuleName: "A",
				}},
			},
		},
	},
	{
		Name: "D",
		Kind: &pbsubstreams.Module_KindMap_{KindMap: &pbsubstreams.Module_KindMap{}},
		Inputs: []*pbsubstreams.Module_Input{
			{
				Input: &pbsubstreams.Module_Input_Store_{Store: &pbsubstreams.Module_Input_Store{
					ModuleName: "B",
				}},
			},
		},
	},
	{
		Name: "E",
		Kind: &pbsubstreams.Module_KindStore_{KindStore: &pbsubstreams.Module_KindStore{}},
		Inputs: []*pbsubstreams.Module_Input{
			{
				Input: &pbsubstreams.Module_Input_Store_{Store: &pbsubstreams.Module_Input_Store{
					ModuleName: "C",
				}},
			},
		},

		//Inputs: []*manifest.Input{
		//	{
		//		Map:  "C",
		//		Name: "store:C",
		//	},
		//	{
		//		Map:  "D",
		//		Name: "map:D",
		//	},
		//},
	},
}

func TestFileWaiter_Wait(t *testing.T) {
	graph, err := manifest.NewModuleGraph(testModules)
	_ = graph
	assert.NoError(t, err)

	tests := []struct {
		name          string
		graph         *manifest.ModuleGraph
		builders      []*Builder
		targetBlock   uint64
		expectedError bool
	}{
		{
			name:  "files all present",
			graph: graph,
			builders: []*Builder{
				mustGetWaiterTestStore("B", "module.hash.1", func(ctx context.Context, prefix, ignoreSuffix string, max int) ([]string, error) {
					files := map[string][]string{
						"0000001000": {"0000001000-0000000000.kv"},
						"0000002000": {"0000002000-0000001000.partial"},
						"0000003000": {"0000003000-0000002000.partial"},
					}
					return files[prefix], nil

				}),
				mustGetWaiterTestStore("C", "module.hash.1", func(ctx context.Context, prefix, ignoreSuffix string, max int) ([]string, error) {
					files := map[string][]string{
						"0000001000": {"0000001000-0000000000.kv"},
						"0000002000": {"0000002000-0000001000.partial"},
						"0000003000": {"0000003000-0000002000.partial"},
					}
					return files[prefix], nil

				}),
			},
			targetBlock:   3000,
			expectedError: false,
		},
		{
			name:  "file missing on one store",
			graph: graph,
			builders: []*Builder{
				mustGetWaiterTestStore("B", "module.hash.1", func(ctx context.Context, prefix, ignoreSuffix string, max int) ([]string, error) {
					files := map[string][]string{
						"0000001000": {"0000001000-0000000000.kv"},
						"0000002000": {"0000002000-0000001000.partial"},
						"0000003000": {"0000003000-0000002000.partial"},
					}

					return files[prefix], nil

				}),
				mustGetWaiterTestStore("C", "module.hash.1", func(ctx context.Context, prefix, ignoreSuffix string, max int) ([]string, error) {
					files := map[string][]string{
						"0000001000": {"0000001000-0000000000.kv"},
						"0000003000": {"0000003000-0000002000.partial"},
					}
					return files[prefix], nil
				}),
			},
			targetBlock:   3000,
			expectedError: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			waiter := NewFileWaiter(test.targetBlock, test.builders)

			ctx, cancel := context.WithTimeout(context.Background(), 250*time.Millisecond)
			defer cancel()

			err = waiter.Wait(ctx, test.targetBlock, 1000)
			if test.expectedError {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
		})
	}
}

func Test_pathToState(t *testing.T) {
	tests := []struct {
		name             string
		builder          *Builder
		storeName        string
		moduleStartBlock uint64
		targetBlock      uint64
		expectedOk       bool
		expectedFiles    []string
		expectedError    bool
	}{
		{
			name:      "happy path",
			storeName: "A",
			builder: mustGetWaiterTestStore("A", "module.hash.1", func(ctx context.Context, prefix, ignoreSuffix string, max int) ([]string, error) {
				files := map[string][]string{
					"0000001000": {"0000001000-0000000000.kv"},
					"0000002000": {"0000002000-0000001000.partial"},
					"0000003000": {"0000003000-0000002000.partial"},
				}
				return files[prefix], nil

			}),
			moduleStartBlock: 0,
			targetBlock:      3000,
			expectedOk:       true,
			expectedFiles:    []string{"0000001000-0000000000.kv", "0000002000-0000001000.partial", "0000003000-0000002000.partial"},
			expectedError:    false,
		},
		{
			name:      "happy path all partial with start block",
			storeName: "A",
			builder: mustGetWaiterTestStore("A", "module.hash.1", func(ctx context.Context, prefix, ignoreSuffix string, max int) ([]string, error) {
				files := map[string][]string{
					"0000002000": {"0000002000-0000001000.partial"},
					"0000003000": {"0000003000-0000002000.partial"},
				}
				return files[prefix], nil

			}),
			moduleStartBlock: 1000,
			targetBlock:      3000,
			expectedOk:       true,
			expectedFiles:    []string{"0000002000-0000001000.partial", "0000003000-0000002000.partial"},
			expectedError:    false,
		},
		{
			name:      "happy path take shortest path",
			storeName: "A",
			builder: mustGetWaiterTestStore("A", "module.hash.1", func(ctx context.Context, prefix, ignoreSuffix string, max int) ([]string, error) {
				files := map[string][]string{
					"0000002000": {"0000002000-0000001000.partial"},
					"0000003000": {"0000003000-0000000000.kv", "module.hash.1-0000003000-0000002000.partial"},
				}
				return files[prefix], nil
			}),
			moduleStartBlock: 1000,
			targetBlock:      3000,
			expectedOk:       true,
			expectedFiles:    []string{"0000003000-0000000000.kv"},
			expectedError:    false,
		},
		{
			name:      "happy path take shortest path part 2",
			storeName: "A",
			builder: mustGetWaiterTestStore("A", "module.hash.1", func(ctx context.Context, prefix, ignoreSuffix string, max int) ([]string, error) {
				files := map[string][]string{
					"0000002000": {"0000002000-0000001000.partial"},
					"0000003000": {"0000003000-0000002000.partial", "0000003000-0000000000.kv"},
				}
				return files[prefix], nil
			}),
			moduleStartBlock: 1000,
			targetBlock:      3000,
			expectedOk:       true,
			expectedFiles:    []string{"0000003000-0000000000.kv"},
			expectedError:    false,
		},
		{
			name:      "conflicting partial files",
			storeName: "A",
			builder: mustGetWaiterTestStore("A", "module.hash.1", func(ctx context.Context, prefix, ignoreSuffix string, max int) ([]string, error) {
				files := map[string][]string{
					"0000001000": {"0000001000-0000000000.kv"},
					"0000002000": {"0000002000-1000.partial"},
					"0000003000": {"0000003000-1000.partial", "module.hash.1-0000003000-0000002000.partial"},
				}
				return files[prefix], nil
			}),
			moduleStartBlock: 0,
			targetBlock:      3000,
			expectedOk:       true,
			expectedFiles:    nil,
			expectedError:    true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			files, err := pathToState(context.TODO(), test.storeName, test.builder.Store, test.targetBlock, test.moduleStartBlock)
			assert.Equal(t, test.expectedFiles, files)
			assert.Equal(t, test.expectedError, err != nil)
		})
	}
}

func mustGetWaiterTestStore(moduleName string, moduleHash string, listFilesFunc func(ctx context.Context, prefix, ignoreSuffix string, max int) ([]string, error)) *Builder {
	mockDStore := &dstore.MockStore{
		ListFilesFunc: listFilesFunc,
	}
	return &Builder{Name: moduleName, ModuleHash: moduleHash, Store: mockDStore}
}
