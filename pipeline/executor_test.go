package pipeline

import (
	"testing"

	"github.com/streamingfast/substreams/pipeline/outputs"
	"github.com/stretchr/testify/require"
)

func TestOptimizeExecutors(t *testing.T) {
	tests := []struct {
		name                                string
		requestedOutputStores               []string
		outputCache                         map[string]*outputs.OutputCache
		moduleExecutors                     []ModuleExecutor
		expectedModuleExecutorsOutputStores []ModuleExecutor
	}{
		{
			name:                  "tests_2_stores",
			requestedOutputStores: []string{"store1", "store4"},
			outputCache: map[string]*outputs.OutputCache{
				"store1": {
					ModuleName: "store1",
					New:        false,
				},
				"store4": {
					ModuleName: "store4",
					New:        false,
				},
			},
			moduleExecutors: []ModuleExecutor{
				&StoreModuleExecutor{
					BaseExecutor: &BaseExecutor{
						moduleName: "store1",
					},
					outputStore: nil,
				},
				&StoreModuleExecutor{
					BaseExecutor: &BaseExecutor{
						moduleName: "store2",
					},
					outputStore: nil,
				},
				&StoreModuleExecutor{
					BaseExecutor: &BaseExecutor{
						moduleName: "store3",
					},
					outputStore: nil,
				},
				&StoreModuleExecutor{
					BaseExecutor: &BaseExecutor{
						moduleName: "store4",
					},
					outputStore: nil,
				},
			},
			expectedModuleExecutorsOutputStores: []ModuleExecutor{
				&StoreModuleExecutor{
					BaseExecutor: &BaseExecutor{
						moduleName: "store1",
					},
					outputStore: nil,
				},
				&StoreModuleExecutor{
					BaseExecutor: &BaseExecutor{
						moduleName: "store4",
					},
					outputStore: nil,
				},
			},
		},
		{
			name:                  "tests-1-store",
			requestedOutputStores: []string{"store1"},
			outputCache: map[string]*outputs.OutputCache{
				"store1": {
					ModuleName: "store1",
					New:        false,
				},
			},
			moduleExecutors: []ModuleExecutor{
				&StoreModuleExecutor{
					BaseExecutor: &BaseExecutor{
						moduleName: "store1",
					},
					outputStore: nil,
				},
				&MapperModuleExecutor{
					BaseExecutor: &BaseExecutor{
						moduleName: "map1",
					},
				},
				&MapperModuleExecutor{
					BaseExecutor: &BaseExecutor{
						moduleName: "map2",
					},
				},
				&StoreModuleExecutor{
					BaseExecutor: &BaseExecutor{
						moduleName: "store2",
					},
					outputStore: nil,
				},
			},
			expectedModuleExecutorsOutputStores: []ModuleExecutor{
				&StoreModuleExecutor{
					BaseExecutor: &BaseExecutor{
						moduleName: "store1",
					},
					outputStore: nil,
				},
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			optimizedModuleExecutors, _ := OptimizeExecutors(test.outputCache, test.moduleExecutors, test.requestedOutputStores)
			require.Equal(t, test.expectedModuleExecutorsOutputStores, optimizedModuleExecutors)
		})
	}
}
