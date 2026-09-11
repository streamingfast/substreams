package exec

import (
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
)

func TestNew() *Graph {
	return &Graph{
		outputModule: &pbsubstreams.Module{
			Name: "",
		},
	}
}

// TestGraphStores returns a graph of two stages: the stores, all in one layer, then an
// output mapper starting at the lowest initial block of the stores.
func TestGraphStores(stores ...*pbsubstreams.Module) *Graph {
	lowest := stores[0].InitialBlock
	initBlocks := make(map[string]uint64, len(stores)+1)
	for _, st := range stores {
		lowest = min(lowest, st.InitialBlock)
		initBlocks[st.Name] = st.InitialBlock
	}
	output := &pbsubstreams.Module{
		Name:         "output",
		Kind:         &pbsubstreams.Module_KindMap_{KindMap: &pbsubstreams.Module_KindMap{}},
		InitialBlock: lowest,
	}
	initBlocks[output.Name] = lowest

	return &Graph{
		lowestInitBlock:   lowest,
		modulesInitBlocks: initBlocks,
		outputModule:      output,
		stagedUsedModules: ExecutionStages{
			{LayerModules(stores)},
			{{output}},
		},
	}
}

func TestGraphStagedModules(initialBlock1, ib2, ib3, ib4, ib5 uint64) *Graph {
	lowest := initialBlock1
	lowest = min(lowest, ib2)
	lowest = min(lowest, ib3)
	lowest = min(lowest, ib4)
	lowest = min(lowest, ib5)
	return &Graph{
		lowestInitBlock: lowest,
		stagedUsedModules: ExecutionStages{
			{
				{
					&pbsubstreams.Module{
						Kind:         &pbsubstreams.Module_KindMap_{KindMap: &pbsubstreams.Module_KindMap{}},
						InitialBlock: initialBlock1,
					},
				}, {
					&pbsubstreams.Module{
						Kind:         &pbsubstreams.Module_KindStore_{KindStore: &pbsubstreams.Module_KindStore{}},
						InitialBlock: ib2,
					},
				},
			},
			{

				{
					&pbsubstreams.Module{
						Kind:         &pbsubstreams.Module_KindMap_{KindMap: &pbsubstreams.Module_KindMap{}},
						InitialBlock: ib3,
					},
				}, {
					&pbsubstreams.Module{
						Kind:         &pbsubstreams.Module_KindStore_{KindStore: &pbsubstreams.Module_KindStore{}},
						InitialBlock: ib4,
					},
				},
			},
			{
				{
					&pbsubstreams.Module{
						Kind:         &pbsubstreams.Module_KindMap_{KindMap: &pbsubstreams.Module_KindMap{}},
						InitialBlock: ib5,
					},
				},
			},
		},
	}
}
