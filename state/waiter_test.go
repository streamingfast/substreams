package state

import (
	"context"
	"fmt"
	"github.com/test-go/testify/require"
	"testing"
	"time"

	pbtransform "github.com/streamingfast/substreams/pb/sf/substreams/transform/v1"

	"github.com/streamingfast/dstore"
	"github.com/streamingfast/substreams/manifest"
	"github.com/stretchr/testify/assert"
)

func TestKVRegex(t *testing.T) {
	filename := "test-0000012345-0000000000.kv"
	res := fullKVRegex.FindAllStringSubmatch(filename, 1)

	assert.Greater(t, len(res[0]), 0)
	assert.Equal(t, "0000012345", res[0][1])
	assert.Equal(t, "0000000000", res[0][2])
}

func TestPartialRegex(t *testing.T) {
	filename := "test-0000012345-0000010345.partial"
	res := partialKVRegex.FindAllStringSubmatch(filename, 1)
	assert.Greater(t, len(res[0]), 0)
	assert.Equal(t, "0000012345", res[0][1])
	assert.Equal(t, "0000010345", res[0][2])
}

var testModules = []*pbtransform.Module{
	{
		Name:   "A",
		Kind:   &pbtransform.Module_KindMap{KindMap: &pbtransform.KindMap{}},
		Inputs: nil,
	},
	{
		Name: "B",
		Kind: &pbtransform.Module_KindStore{KindStore: &pbtransform.KindStore{}},
		Inputs: []*pbtransform.Input{
			{
				Input: &pbtransform.Input_Store{Store: &pbtransform.InputStore{
					ModuleName: "A",
				}},
			},
		},
	},
	{
		Name: "C",
		Kind: &pbtransform.Module_KindStore{KindStore: &pbtransform.KindStore{}},
		Inputs: []*pbtransform.Input{
			{
				Input: &pbtransform.Input_Store{Store: &pbtransform.InputStore{
					ModuleName: "A",
				}},
			},
		},
	},
	{
		Name: "D",
		Kind: &pbtransform.Module_KindMap{KindMap: &pbtransform.KindMap{}},
		Inputs: []*pbtransform.Input{
			{
				Input: &pbtransform.Input_Store{Store: &pbtransform.InputStore{
					ModuleName: "B",
				}},
			},
		},
	},
	{
		Name: "E",
		Kind: &pbtransform.Module_KindStore{KindStore: &pbtransform.KindStore{}},
		Inputs: []*pbtransform.Input{
			{
				Input: &pbtransform.Input_Store{Store: &pbtransform.InputStore{
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
		stores        []*Store
		targetBlock   uint64
		expectedError bool
	}{
		{
			name:  "files all present",
			graph: graph,
			stores: []*Store{
				mustGetWaiterTestStore("B", "module.hash.1", func(ctx context.Context, prefix, ignoreSuffix string, max int) ([]string, error) {
					files := map[string][]string{
						"B-0000001000": {"B-0000001000-0000000000.kv"},
						"B-0000002000": {"B-0000002000-0000001000.partial"},
						"B-0000003000": {"B-0000003000-0000002000.partial"},
					}
					return files[prefix], nil

				}),
				mustGetWaiterTestStore("C", "module.hash.1", func(ctx context.Context, prefix, ignoreSuffix string, max int) ([]string, error) {
					files := map[string][]string{
						"C-0000001000": {"C-0000001000-0000000000.kv"},
						"C-0000002000": {"C-0000002000-0000001000.partial"},
						"C-0000003000": {"C-0000003000-0000002000.partial"},
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
			stores: []*Store{
				mustGetWaiterTestStore("B", "module.hash.1", func(ctx context.Context, prefix, ignoreSuffix string, max int) ([]string, error) {
					files := map[string][]string{
						"B-0000001000": {"B-0000001000-0000000000.kv"},
						"B-0000002000": {"B-0000002000-0000001000.partial"},
						"B-0000003000": {"B-0000003000-0000002000.partial"},
					}

					return files[prefix], nil

				}),
				mustGetWaiterTestStore("C", "module.hash.1", func(ctx context.Context, prefix, ignoreSuffix string, max int) ([]string, error) {
					files := map[string][]string{
						"C-0000001000": {"C-0000001000-0000000000.kv"},
						"C-0000003000": {"C-0000003000-0000002000.partial"},
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
			waiter := NewFileWaiter(test.targetBlock, test.stores)

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
		store            *Store
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
			store: mustGetWaiterTestStore("A", "module.hash.1", func(ctx context.Context, prefix, ignoreSuffix string, max int) ([]string, error) {
				files := map[string][]string{
					"A-0000001000": {"A-0000001000-0000000000.kv"},
					"A-0000002000": {"A-0000002000-0000001000.partial"},
					"A-0000003000": {"A-0000003000-0000002000.partial"},
				}
				return files[prefix], nil

			}),
			moduleStartBlock: 0,
			targetBlock:      3000,
			expectedOk:       true,
			expectedFiles:    []string{"A-0000001000-0000000000.kv", "A-0000002000-0000001000.partial", "A-0000003000-0000002000.partial"},
			expectedError:    false,
		},
		{
			name:      "happy path all partial with start block",
			storeName: "A",
			store: mustGetWaiterTestStore("A", "module.hash.1", func(ctx context.Context, prefix, ignoreSuffix string, max int) ([]string, error) {
				files := map[string][]string{
					"A-0000002000": {"A-0000002000-0000001000.partial"},
					"A-0000003000": {"A-0000003000-0000002000.partial"},
				}
				return files[prefix], nil

			}),
			moduleStartBlock: 1000,
			targetBlock:      3000,
			expectedOk:       true,
			expectedFiles:    []string{"A-0000002000-0000001000.partial", "A-0000003000-0000002000.partial"},
			expectedError:    false,
		},
		{
			name:      "happy path take shortest path",
			storeName: "A",
			store: mustGetWaiterTestStore("A", "module.hash.1", func(ctx context.Context, prefix, ignoreSuffix string, max int) ([]string, error) {
				files := map[string][]string{
					"A-0000002000": {"A-0000002000-0000001000.partial"},
					"A-0000003000": {"A-0000003000-0000000000.kv", "module.hash.1-A-0000003000-0000002000.partial"},
				}
				return files[prefix], nil
			}),
			moduleStartBlock: 1000,
			targetBlock:      3000,
			expectedOk:       true,
			expectedFiles:    []string{"A-0000003000-0000000000.kv"},
			expectedError:    false,
		},
		{
			name:      "happy path take shortest path part 2",
			storeName: "A",
			store: mustGetWaiterTestStore("A", "module.hash.1", func(ctx context.Context, prefix, ignoreSuffix string, max int) ([]string, error) {
				files := map[string][]string{
					"A-0000002000": {"A-0000002000-0000001000.partial"},
					"A-0000003000": {"A-0000003000-0000002000.partial", "A-0000003000-0000000000.kv"},
				}
				return files[prefix], nil
			}),
			moduleStartBlock: 1000,
			targetBlock:      3000,
			expectedOk:       true,
			expectedFiles:    []string{"A-0000003000-0000000000.kv"},
			expectedError:    false,
		},
		{
			name:      "conflicting partial files",
			storeName: "A",
			store: mustGetWaiterTestStore("A", "module.hash.1", func(ctx context.Context, prefix, ignoreSuffix string, max int) ([]string, error) {
				files := map[string][]string{
					"A-0000001000": {"A-0000001000-0000000000.kv"},
					"A-0000002000": {"A-0000002000-1000.partial"},
					"A-0000003000": {"A-0000003000-1000.partial", "module.hash.1-A-0000003000-0000002000.partial"},
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
			files, err := pathToState(context.TODO(), test.store, test.targetBlock, test.moduleStartBlock)
			assert.Equal(t, test.expectedFiles, files)
			assert.Equal(t, test.expectedError, err != nil)
		})
	}
}

func mustGetWaiterTestStore(moduleName string, moduleHash string, listFilesFunc func(ctx context.Context, prefix, ignoreSuffix string, max int) ([]string, error)) *Store {
	mockDStore := &dstore.MockStore{
		ListFilesFunc: listFilesFunc,
	}
	s, err := NewStore(moduleName, moduleHash, 0, mockDStore)
	if err != nil {
		panic(fmt.Sprintf("faild to create mock store: %s", err))
	}
	return s
}
