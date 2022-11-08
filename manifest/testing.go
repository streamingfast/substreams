package manifest

import (
	"github.com/mitchellh/go-testing-interface"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
	"github.com/stretchr/testify/require"
)

var zero = uint64(0)
var five = uint64(5)
var ten = uint64(10)
var twenty = uint64(20)
var thirty = uint64(30)

// NewTestModules can be used in foreign packages for their test suite
func NewTestModules() []*pbsubstreams.Module {
	return []*pbsubstreams.Module{
		{
			Name:         "As",
			InitialBlock: zero,
			Kind:         &pbsubstreams.Module_KindStore_{KindStore: &pbsubstreams.Module_KindStore{}},
		},
		{
			Name:         "Am",
			InitialBlock: zero,
			Kind:         &pbsubstreams.Module_KindMap_{KindMap: &pbsubstreams.Module_KindMap{}},
		},
		{
			Name:         "B",
			InitialBlock: ten,
			Kind:         &pbsubstreams.Module_KindStore_{KindStore: &pbsubstreams.Module_KindStore{}},
			Inputs: []*pbsubstreams.Module_Input{
				{
					Input: &pbsubstreams.Module_Input_Map_{Map: &pbsubstreams.Module_Input_Map{
						ModuleName: "Am",
					}},
				},
			},
		},
		{
			Name:         "C",
			InitialBlock: zero,
			Kind:         &pbsubstreams.Module_KindMap_{KindMap: &pbsubstreams.Module_KindMap{}},
			Inputs: []*pbsubstreams.Module_Input{
				{
					Input: &pbsubstreams.Module_Input_Store_{Store: &pbsubstreams.Module_Input_Store{
						ModuleName: "As",
					}},
				},
			},
		},
		{
			Name:         "D",
			InitialBlock: zero,
			Kind:         &pbsubstreams.Module_KindMap_{KindMap: &pbsubstreams.Module_KindMap{}},
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
			InitialBlock: five,
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

func TestReadManifest(t testing.T, manifestPath string) *pbsubstreams.Package {
	t.Helper()

	manifestReader := NewReader(manifestPath)
	pkg, err := manifestReader.Read()
	require.NoError(t, err)
	return pkg
}
