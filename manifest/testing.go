package manifest

import pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"

var ten = uint64(10)
var twenty = uint64(20)
var thirty = uint64(30)

func NewTestModules() []*pbsubstreams.Module {
	return []*pbsubstreams.Module{
		{
			Name: "A",
		},
		{
			Name:         "B",
			InitialBlock: ten,
			Kind:         &pbsubstreams.Module_KindStore_{KindStore: &pbsubstreams.Module_KindStore{}},
			Inputs: []*pbsubstreams.Module_Input{
				{
					Input: &pbsubstreams.Module_Input_Map_{Map: &pbsubstreams.Module_Input_Map{
						ModuleName: "A",
					}},
				},
			},
		},
		{
			Name:         "C",
			InitialBlock: twenty,
			Kind:         &pbsubstreams.Module_KindMap_{KindMap: &pbsubstreams.Module_KindMap{}},
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
			Name:         "E",
			InitialBlock: ten,
			Kind:         &pbsubstreams.Module_KindStore_{KindStore: &pbsubstreams.Module_KindStore{}},
			Inputs: []*pbsubstreams.Module_Input{
				{
					Input: &pbsubstreams.Module_Input_Map_{Map: &pbsubstreams.Module_Input_Map{
						ModuleName: "C",
					}},
				},
			},
		},
		{
			Name: "F",
			Kind: &pbsubstreams.Module_KindStore_{KindStore: &pbsubstreams.Module_KindStore{}},
			Inputs: []*pbsubstreams.Module_Input{
				{
					Input: &pbsubstreams.Module_Input_Store_{Store: &pbsubstreams.Module_Input_Store{
						ModuleName: "C",
					}},
				},
			},
		},
		{
			Name: "G",
			Kind: &pbsubstreams.Module_KindStore_{KindStore: &pbsubstreams.Module_KindStore{}},
			Inputs: []*pbsubstreams.Module_Input{
				{
					Input: &pbsubstreams.Module_Input_Map_{Map: &pbsubstreams.Module_Input_Map{
						ModuleName: "D",
					}},
				},
				{
					Input: &pbsubstreams.Module_Input_Store_{Store: &pbsubstreams.Module_Input_Store{
						ModuleName: "E",
					}},
				},
			},
		},
		{
			Name: "K",
			Kind: &pbsubstreams.Module_KindStore_{KindStore: &pbsubstreams.Module_KindStore{}},
			Inputs: []*pbsubstreams.Module_Input{
				{
					Input: &pbsubstreams.Module_Input_Store_{Store: &pbsubstreams.Module_Input_Store{
						ModuleName: "G",
					}},
				},
			},
		},
		{
			Name:   "H",
			Kind:   &pbsubstreams.Module_KindMap_{KindMap: &pbsubstreams.Module_KindMap{}},
			Inputs: nil,
		},
	}

}
