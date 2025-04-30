package exec

import (
	"github.com/streamingfast/substreams/manifest"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
)

func TestNew() *Graph {
	return &Graph{
		outputModule: &pbsubstreams.Module{
			Name: "",
		},
		moduleHashes: manifest.NewModuleHashes(),
	}
}

func TestSimpleHashes(moduleNames []string) *Graph {
	manifest.TestUseSimpleHash = true
	hashes := manifest.NewModuleHashes()

	var modules []*pbsubstreams.Module

	for _, modName := range moduleNames {
		modules = append(modules, &pbsubstreams.Module{
			Name: modName,
		})
	}

	for _, mod := range modules {
		hashes.HashModule(&pbsubstreams.Modules{Modules: modules}, mod, &manifest.ModuleGraph{})
	}

	return &Graph{
		outputModule: &pbsubstreams.Module{
			Name: "",
		},
		moduleHashes: hashes,
	}
}

func TestGraphStagedModules(initialBlock1, ib2, ib3, ib4, ib5 uint64) *Graph {
	lowest := initialBlock1
	lowest = min(lowest, ib2)
	lowest = min(lowest, ib3)
	lowest = min(lowest, ib4)
	lowest = min(lowest, ib5)
	hashes := manifest.NewModuleHashes()
	hashes.HashModule(nil, &pbsubstreams.Module{Name: "mod1"}, nil)
	hashes.HashModule(nil, &pbsubstreams.Module{Name: "mod2"}, nil)
	hashes.HashModule(nil, &pbsubstreams.Module{Name: "mod3"}, nil)
	hashes.HashModule(nil, &pbsubstreams.Module{Name: "mod4"}, nil)
	hashes.HashModule(nil, &pbsubstreams.Module{Name: "mod5"}, nil)
	return &Graph{
		lowestInitBlock: lowest,
		moduleHashes:    hashes,
		stagedUsedModules: ExecutionStages{
			{
				{
					&pbsubstreams.Module{
						Name:         "mod1",
						Kind:         &pbsubstreams.Module_KindMap_{KindMap: &pbsubstreams.Module_KindMap{}},
						InitialBlock: initialBlock1,
					},
				}, {
					&pbsubstreams.Module{
						Name:         "mod2",
						Kind:         &pbsubstreams.Module_KindStore_{KindStore: &pbsubstreams.Module_KindStore{}},
						InitialBlock: ib2,
					},
				},
			},
			{

				{
					&pbsubstreams.Module{
						Name:         "mod3",
						Kind:         &pbsubstreams.Module_KindMap_{KindMap: &pbsubstreams.Module_KindMap{}},
						InitialBlock: ib3,
					},
				}, {
					&pbsubstreams.Module{
						Name:         "mod4",
						Kind:         &pbsubstreams.Module_KindStore_{KindStore: &pbsubstreams.Module_KindStore{}},
						InitialBlock: ib4,
					},
				},
			},
			{
				{
					&pbsubstreams.Module{
						Name:         "mod5",
						Kind:         &pbsubstreams.Module_KindMap_{KindMap: &pbsubstreams.Module_KindMap{}},
						InitialBlock: ib5,
					},
				},
			},
		},
	}
}
