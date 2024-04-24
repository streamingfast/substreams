package exec

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/test-go/testify/require"

	pbsubstreamsrpc "github.com/streamingfast/substreams/pb/sf/substreams/rpc/v2"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
)

func TestGraph_computeStages(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		expect string
	}{
		{
			name:   "some graph",
			input:  "Sa Mb Mc Sd:Sa,Mb Me:Sd",
			expect: "[[Sa]] [[Mb Mc] [Sd]] [[Me]]",
		},
		{
			name:   "other graph",
			input:  "Ma Mb:Ma Sc:Mb",
			expect: "[[Ma] [Mb] [Sc]]",
		},
		{
			name:   "third graph",
			input:  "Ma Mb:Ma Sc:Mb Md:Sc Se:Md",
			expect: "[[Ma] [Mb] [Sc]] [[Md] [Se]]",
		},
		{
			name:   "fourth graph",
			input:  "Ma Mb:Ma Sc:Mb Md:Sc Se:Md,Sg Mf:Ma Sg:Mf",
			expect: "[[Ma] [Mb Mf] [Sc Sg]] [[Md] [Se]]",
		},
		{
			name:   "fifth graph",
			input:  "Ma Mb:Ma Sc:Mb Md:Sc Se:Md,Sg Mf:Ma Sg:Mf Mh:Se,Ma",
			expect: "[[Ma] [Mb Mf] [Sc Sg]] [[Md] [Se]] [[Mh]]",
		},
		{
			name:   "sixth graph (block index impl)",
			input:  "Ia Mb Md:Sc,Ia Sc:Mb",
			expect: "[[Ia Mb] [Sc]] [[Md]]",
		},
		{
			name:   "seventh graph (block index impl)",
			input:  "Ia Mb Md:Sc,Ia Sc:Mb Me:Sc Mf:Me,Sg,Ia Sg:Md,Me",
			expect: "[[Ia Mb] [Sc]] [[Md Me] [Sg]] [[Mf]]",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			out, err := computeStages(computeStagesInput(test.input), nil)
			require.NoError(t, err)
			assert.Equal(t, test.expect, computeStagesOutput(out))
		})
	}
}

func computeStagesInput(in string) (out []*pbsubstreams.Module) {
	for _, mod := range strings.Split(in, " ") {
		if mod == "" {
			continue
		}
		params := strings.Split(mod, ":")
		modName := params[0]
		newMod := &pbsubstreams.Module{}
		switch modName[0] {
		case 'S':
			newMod.Kind = &pbsubstreams.Module_KindStore_{KindStore: &pbsubstreams.Module_KindStore{}}
			newMod.Name = modName[1:]
		case 'M':
			newMod.Kind = &pbsubstreams.Module_KindMap_{KindMap: &pbsubstreams.Module_KindMap{}}
			newMod.Name = modName[1:]
		case 'I':
			newMod.Kind = &pbsubstreams.Module_KindBlockIndex_{KindBlockIndex: &pbsubstreams.Module_KindBlockIndex{}}
			newMod.Name = modName[1:]
		default:
			panic("invalid prefix in word: " + modName)
		}
		// we set at least one block source so it doesn't fail the validation
		newMod.Inputs = []*pbsubstreams.Module_Input{
			{Input: &pbsubstreams.Module_Input_Source_{Source: &pbsubstreams.Module_Input_Source{Type: "test.block"}}},
		}
		if len(params) > 1 {
			for _, input := range strings.Split(params[1], ",") {
				inputName := input[1:]

				switch input[0] {
				case 'S':
					newMod.Inputs = append(newMod.Inputs, &pbsubstreams.Module_Input{Input: &pbsubstreams.Module_Input_Store_{Store: &pbsubstreams.Module_Input_Store{ModuleName: inputName}}})
				case 'M':
					newMod.Inputs = append(newMod.Inputs, &pbsubstreams.Module_Input{Input: &pbsubstreams.Module_Input_Map_{Map: &pbsubstreams.Module_Input_Map{ModuleName: inputName}}})
				case 'P':
					newMod.Inputs = append(newMod.Inputs, &pbsubstreams.Module_Input{Input: &pbsubstreams.Module_Input_Params_{}})
				case 'R':
					newMod.Inputs = append(newMod.Inputs, &pbsubstreams.Module_Input{Input: &pbsubstreams.Module_Input_Source_{}})
				case 'I':
					newMod.BlockFilter = &pbsubstreams.Module_BlockFilter{
						Module: inputName,
						Query:  "",
					}
				default:
					panic("invalid input prefix: " + input)
				}
			}
		}
		out = append(out, newMod)
	}
	return
}

func computeStagesOutput(in ExecutionStages) string {
	var level1 []string
	for _, l1 := range in {
		var level2 []string
		for _, l2 := range l1 {
			var level3 []string
			for _, l3 := range l2 {
				modKind := "S"
				if l3.GetKindMap() != nil {
					modKind = "M"
				}
				if l3.GetKindBlockIndex() != nil {
					modKind = "I"
				}
				level3 = append(level3, modKind+l3.Name)
			}
			level2 = append(level2, fmt.Sprintf("%v", level3))
		}
		level1 = append(level1, fmt.Sprintf("%v", level2))
	}
	return strings.Join(level1, " ")
}

func TestGraph_computeSchedulableModules(t *testing.T) {
	tests := []struct {
		name           string
		stores         []*pbsubstreams.Module
		outputModule   *pbsubstreams.Module
		productionMode bool
		expect         []*pbsubstreams.Module
	}{

		{
			name:         "dev mode with output module map",
			stores:       []*pbsubstreams.Module{pbsubstreamsrpc.TestNewStoreModule("store_a"), pbsubstreamsrpc.TestNewStoreModule("store_b")},
			outputModule: pbsubstreamsrpc.TestNewMapModule("map_a"),
			expect:       []*pbsubstreams.Module{pbsubstreamsrpc.TestNewStoreModule("store_a"), pbsubstreamsrpc.TestNewStoreModule("store_b")},
		},
		{
			name:         "dev mode with output module store",
			stores:       []*pbsubstreams.Module{pbsubstreamsrpc.TestNewStoreModule("store_a"), pbsubstreamsrpc.TestNewStoreModule("store_b")},
			outputModule: pbsubstreamsrpc.TestNewStoreModule("store_b"),
			expect:       []*pbsubstreams.Module{pbsubstreamsrpc.TestNewStoreModule("store_a"), pbsubstreamsrpc.TestNewStoreModule("store_b")},
		},
		{
			name:           "prod mode with output module map",
			stores:         []*pbsubstreams.Module{pbsubstreamsrpc.TestNewStoreModule("store_a"), pbsubstreamsrpc.TestNewStoreModule("store_b")},
			outputModule:   pbsubstreamsrpc.TestNewMapModule("map_a"),
			productionMode: true,
			expect:         []*pbsubstreams.Module{pbsubstreamsrpc.TestNewStoreModule("store_a"), pbsubstreamsrpc.TestNewStoreModule("store_b"), pbsubstreamsrpc.TestNewMapModule("map_a")},
		},
		{
			name:           "prod mode with output module store",
			stores:         []*pbsubstreams.Module{pbsubstreamsrpc.TestNewStoreModule("store_a"), pbsubstreamsrpc.TestNewStoreModule("store_b")},
			outputModule:   pbsubstreamsrpc.TestNewStoreModule("store_b"),
			productionMode: true,
			expect:         []*pbsubstreams.Module{pbsubstreamsrpc.TestNewStoreModule("store_a"), pbsubstreamsrpc.TestNewStoreModule("store_b")},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			out := computeSchedulableModules(test.stores, test.outputModule, test.productionMode)

			assert.Equal(t, test.expect, out)
		})
	}
}
