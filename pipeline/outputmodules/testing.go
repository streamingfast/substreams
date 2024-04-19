package outputmodules

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
