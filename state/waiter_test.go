package state

import (
	"context"
	"fmt"
	"sort"
	"testing"
	"time"

	"github.com/streamingfast/dstore"
	"github.com/streamingfast/substreams/manifest"
	"github.com/stretchr/testify/assert"
)

func TestRangeSort(t *testing.T) {
	brs := blockRangeItems{blockRangeItem{start: 2500, end: 3000}, blockRangeItem{start: 0, end: 1300}, blockRangeItem{start: 0, end: 1000}, blockRangeItem{start: 1000, end: 1500}, blockRangeItem{start: 1500, end: 2000}}
	sort.Sort(brs)
	fmt.Println(brs)
}

func TestKVRegex(t *testing.T) {
	filename := "test-12345.kv"
	res := fullKVRegex.FindAllStringSubmatch(filename, 1)

	assert.Greater(t, len(res[0]), 0)
	assert.Equal(t, res[0][1], "12345")
}

func TestPartialRegex(t *testing.T) {
	filename := "test-01234-12345.partial"
	res := partialKVRegex.FindAllStringSubmatch(filename, 1)
	assert.Greater(t, len(res[0]), 0)
	assert.Equal(t, res[0][1], "01234")
	assert.Equal(t, res[0][2], "12345")
}

var testModules = []*manifest.Module{
	{
		Name:   "A",
		Kind:   manifest.ModuleKindMap,
		Inputs: nil,
	},
	{
		Name: "B",
		Kind: manifest.ModuleKindStore,
		Inputs: []*manifest.Input{
			{
				Map:  "A",
				Name: "map:A",
			},
		},
	},
	{
		Name: "C",
		Kind: manifest.ModuleKindStore,
		Inputs: []*manifest.Input{
			{
				Map:  "A",
				Name: "map:A",
			},
		},
	},
	{
		Name: "D",
		Kind: manifest.ModuleKindMap,
		Inputs: []*manifest.Input{
			{
				Map:  "B",
				Name: "store:B",
			},
		},
	},
	{
		Name: "E",
		Kind: manifest.ModuleKindStore,
		Inputs: []*manifest.Input{
			{
				Map:  "C",
				Name: "store:C",
			},
			{
				Map:  "D",
				Name: "map:D",
			},
		},
	},
}

func TestFileWaiter_Wait(t *testing.T) {
	graph, err := manifest.NewModuleGraph(testModules)
	assert.NoError(t, err)

	tests := []struct {
		name          string
		graph         *manifest.ModuleGraph
		factory       FactoryInterface
		targetBlock   uint64
		expectedError bool
	}{
		{
			name:  "files all present",
			graph: graph,
			factory: &TestFactory{
				stores: map[string]*TestStore{
					"B": getWaiterTestStore(func(ctx context.Context, prefix, ignoreSuffix string, f func(filename string) error) error {
						bFiles := []string{"B-1000.kv", "B-1000-2000.partial", "B-2000-3000.partial"}
						for _, bf := range bFiles {
							err := f(bf)
							if err != nil {
								return err
							}
						}
						return nil
					}),
					"C": getWaiterTestStore(func(ctx context.Context, prefix, ignoreSuffix string, f func(filename string) error) error {
						bFiles := []string{"C-1500.kv", "C-1500-3000.partial"}
						for _, bf := range bFiles {
							err := f(bf)
							if err != nil {
								return err
							}
						}
						return nil
					}),
				},
			},
			targetBlock:   3000,
			expectedError: false,
		},
		{
			name:  "file missing on one store",
			graph: graph,
			factory: &TestFactory{
				stores: map[string]*TestStore{
					"B": getWaiterTestStore(func(ctx context.Context, prefix, ignoreSuffix string, f func(filename string) error) error {
						bFiles := []string{"B-1000.kv", "B-1000-2000.partial", "B-2000-3000.partial"}
						for _, bf := range bFiles {
							err := f(bf)
							if err != nil {
								return err
							}
						}
						return nil
					}),
					"C": getWaiterTestStore(func(ctx context.Context, prefix, ignoreSuffix string, f func(filename string) error) error {
						bFiles := []string{"C-1500.kv"} //file missing here
						for _, bf := range bFiles {
							err := f(bf)
							if err != nil {
								return err
							}
						}
						return nil
					}),
				},
			},
			targetBlock:   3000,
			expectedError: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			waiter := NewFileWaiter("E", test.graph, 0, test.factory, test.targetBlock)

			ctx, cancel := context.WithTimeout(context.Background(), 250*time.Millisecond)
			defer cancel()

			err = waiter.Wait(ctx)
			assert.Equal(t, test.expectedError, err != nil)
		})
	}
}

func TestContiguousFilesToTargetBlock(t *testing.T) {
	tests := []struct {
		name             string
		store            StoreInterface
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
			store: getWaiterTestStore(func(ctx context.Context, prefix, ignoreSuffix string, f func(filename string) error) error {
				files := []string{"A-1000.kv", "A-1000-2000.partial", "C-2000-3000.partial"}
				for _, bf := range files {
					err := f(bf)
					if err != nil {
						return err
					}
				}
				return nil
			}),
			moduleStartBlock: 0,
			targetBlock:      3000,
			expectedOk:       true,
			expectedFiles:    []string{"A-1000.kv", "A-1000-2000.partial", "C-2000-3000.partial"},
			expectedError:    false,
		},
		{
			name:      "happy path all partial with start block",
			storeName: "A",
			store: getWaiterTestStore(func(ctx context.Context, prefix, ignoreSuffix string, f func(filename string) error) error {
				files := []string{"A-1000-2000.partial", "C-2000-3000.partial"}
				for _, bf := range files {
					err := f(bf)
					if err != nil {
						return err
					}
				}
				return nil
			}),
			moduleStartBlock: 1000,
			targetBlock:      3000,
			expectedOk:       true,
			expectedFiles:    []string{"A-1000-2000.partial", "C-2000-3000.partial"},
			expectedError:    false,
		},
		{
			name:      "happy path take shortest path",
			storeName: "A",
			store: getWaiterTestStore(func(ctx context.Context, prefix, ignoreSuffix string, f func(filename string) error) error {
				files := []string{"A-3000.kv", "A-1000-2000.partial", "C-2000-3000.partial"}
				for _, bf := range files {
					err := f(bf)
					if err != nil {
						return err
					}
				}
				return nil
			}),
			moduleStartBlock: 1000,
			targetBlock:      3000,
			expectedOk:       true,
			expectedFiles:    []string{"A-3000.kv"},
			expectedError:    false,
		},
		{
			name:      "no fulls",
			storeName: "A",
			store: getWaiterTestStore(func(ctx context.Context, prefix, ignoreSuffix string, f func(filename string) error) error {
				files := []string{"A-1000-2000.partial", "C-2000-3000.partial"}
				for _, bf := range files {
					err := f(bf)
					if err != nil {
						return err
					}
				}
				return nil
			}),
			moduleStartBlock: 0,
			targetBlock:      3000,
			expectedOk:       false,
			expectedFiles:    nil,
			expectedError:    false,
		},
		{
			name:      "no targets",
			storeName: "A",
			store: getWaiterTestStore(func(ctx context.Context, prefix, ignoreSuffix string, f func(filename string) error) error {
				files := []string{"A-1000.kv", "A-1000-2000.partial"}
				for _, bf := range files {
					err := f(bf)
					if err != nil {
						return err
					}
				}
				return nil
			}),
			moduleStartBlock: 0,
			targetBlock:      3000,
			expectedOk:       false,
			expectedFiles:    nil,
			expectedError:    false,
		},
		{
			name:      "no path",
			storeName: "A",
			store: getWaiterTestStore(func(ctx context.Context, prefix, ignoreSuffix string, f func(filename string) error) error {
				files := []string{"A-1000.kv", "C-2000-3000.partial"}
				for _, bf := range files {
					err := f(bf)
					if err != nil {
						return err
					}
				}
				return nil
			}),
			moduleStartBlock: 0,
			targetBlock:      3000,
			expectedOk:       false,
			expectedFiles:    nil,
			expectedError:    false,
		},
		{
			name:      "walk error",
			storeName: "A",
			store: getWaiterTestStore(func(ctx context.Context, prefix, ignoreSuffix string, f func(filename string) error) error {
				files := []string{"invalid.partial"}
				for _, bf := range files {
					err := f(bf)
					if err != nil {
						return err
					}
				}
				return nil
			}),
			moduleStartBlock: 0,
			targetBlock:      3000,
			expectedOk:       false,
			expectedFiles:    nil,
			expectedError:    true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ok, files, err := ContiguousFilesToTargetBlock(context.TODO(), test.storeName, test.store, test.moduleStartBlock, test.targetBlock)
			assert.Equal(t, test.expectedOk, ok)
			assert.Equal(t, test.expectedFiles, files)
			assert.Equal(t, test.expectedError, err != nil)
		})
	}
}

func getWaiterTestStore(walkFunc func(ctx context.Context, prefix, ignoreSuffix string, f func(filename string) error) error) *TestStore {
	mockDStore := &dstore.MockStore{WalkFunc: walkFunc}
	return &TestStore{
		MockStore: mockDStore,
	}
}
