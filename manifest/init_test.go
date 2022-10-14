package manifest

import (
	"encoding/json"
	"testing"

	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

func assertProtoEqual(t *testing.T, expected proto.Message, actual proto.Message) {
	t.Helper()

	if !proto.Equal(expected, actual) {
		expectedAsJSON, err := protojson.Marshal(expected)
		require.NoError(t, err)

		actualAsJSON, err := protojson.Marshal(actual)
		require.NoError(t, err)

		expectedAsMap := map[string]interface{}{}
		err = json.Unmarshal(expectedAsJSON, &expectedAsMap)
		require.NoError(t, err)

		actualAsMap := map[string]interface{}{}
		err = json.Unmarshal(actualAsJSON, &actualAsMap)
		require.NoError(t, err)

		// We use equal is not equal above so we get a good diff, if the first condition failed, the second will also always
		// fail which is what we want here
		assert.Equal(t, expectedAsMap, actualAsMap)
	}
}

var zero = uint64(0)
var five = uint64(5)
var ten = uint64(10)
var twenty = uint64(20)
var thirty = uint64(30)

func NewTestModules() []*pbsubstreams.Module {
	return []*pbsubstreams.Module{
		{
			Name:         "A",
			InitialBlock: zero,
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
			InitialBlock: zero,
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
