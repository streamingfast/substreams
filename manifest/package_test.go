package manifest

import (
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestHandleUseModules(t *testing.T) {
	cases := []struct {
		name                  string
		pkg                   *pbsubstreams.Package
		manifest              *Manifest
		expectedOutputModules []*pbsubstreams.Module
		expectedError         string
	}{
		{
			name: "sunny path",
			pkg: &pbsubstreams.Package{
				ProtoFiles: nil,
				Version:    0,
				Modules: &pbsubstreams.Modules{Modules: []*pbsubstreams.Module{
					{
						Name:             "use_module",
						Kind:             nil,
						BinaryIndex:      0,
						BinaryEntrypoint: "use_module",
						Inputs:           []*pbsubstreams.Module_Input{{Input: &pbsubstreams.Module_Input_Map_{Map: &pbsubstreams.Module_Input_Map{ModuleName: "B"}}}},
						InitialBlock:     5,
					},
					{
						Name:             "B",
						Kind:             &pbsubstreams.Module_KindMap_{KindMap: &pbsubstreams.Module_KindMap{OutputType: "proto:sf.database.v1.changes"}},
						BinaryIndex:      0,
						BinaryEntrypoint: "B",
						Inputs:           []*pbsubstreams.Module_Input{{Input: &pbsubstreams.Module_Input_Source_{Source: &pbsubstreams.Module_Input_Source{Type: "clock"}}}},
						Output:           &pbsubstreams.Module_Output{Type: "proto:sf.database.v1.changes"},
					},
					{
						Name:             "dbout_to_graphout",
						Kind:             &pbsubstreams.Module_KindMap_{KindMap: &pbsubstreams.Module_KindMap{OutputType: "proto:sf.entity.v1.changes"}},
						BinaryIndex:      1,
						BinaryEntrypoint: "dbout_to_graphout",
						Inputs:           []*pbsubstreams.Module_Input{{Input: &pbsubstreams.Module_Input_Map_{Map: &pbsubstreams.Module_Input_Map{ModuleName: "example_dbout"}}}},
						Output:           &pbsubstreams.Module_Output{Type: "proto:sf.entity.v1.changes"},
					},
					{
						Name:             "example_dbout",
						Kind:             &pbsubstreams.Module_KindMap_{KindMap: &pbsubstreams.Module_KindMap{OutputType: "proto:sf.database.v1.changes"}},
						BinaryIndex:      1,
						BinaryEntrypoint: "example_dbout",
						Inputs:           []*pbsubstreams.Module_Input{{Input: &pbsubstreams.Module_Input_Source_{Source: &pbsubstreams.Module_Input_Source{Type: "block"}}}},
						Output:           &pbsubstreams.Module_Output{Type: "proto:sf.database.v1.changes"},
					},
				},
				},
			},
			manifest: &Manifest{
				Modules: []*Module{
					{Name: "use_module", Kind: "map", Inputs: []*Input{{Source: "proto:sf.database.v1.changes"}}, Use: "dbout_to_graphout"},
					{Name: "B", Kind: "map", Inputs: []*Input{{Source: "clock"}}, Output: StreamOutput{Type: "proto:sf.database.v1.changes"}},
					{Name: "dbout_to_graphout", Kind: "map", Inputs: []*Input{{Source: "proto:sf.database.v1.changes"}}},
					{Name: "example_dbout", Kind: "map", Inputs: []*Input{{Source: "block"}}, Output: StreamOutput{Type: "proto:sf.database.v1.changes"}},
				},
			},
			expectedOutputModules: []*pbsubstreams.Module{
				{
					Name:             "use_module",
					Kind:             &pbsubstreams.Module_KindMap_{KindMap: &pbsubstreams.Module_KindMap{OutputType: "proto:sf.entity.v1.changes"}},
					BinaryIndex:      1,
					BinaryEntrypoint: "dbout_to_graphout",
					Inputs:           []*pbsubstreams.Module_Input{{Input: &pbsubstreams.Module_Input_Map_{Map: &pbsubstreams.Module_Input_Map{ModuleName: "B"}}}},
					InitialBlock:     5,
					Output:           &pbsubstreams.Module_Output{Type: "proto:sf.entity.v1.changes"},
				},
				{
					Name:             "B",
					Kind:             &pbsubstreams.Module_KindMap_{KindMap: &pbsubstreams.Module_KindMap{OutputType: "proto:sf.database.v1.changes"}},
					BinaryIndex:      0,
					BinaryEntrypoint: "B",
					Inputs:           []*pbsubstreams.Module_Input{{Input: &pbsubstreams.Module_Input_Source_{Source: &pbsubstreams.Module_Input_Source{Type: "clock"}}}},
					Output:           &pbsubstreams.Module_Output{Type: "proto:sf.database.v1.changes"},
				},
				{
					Name:             "dbout_to_graphout",
					Kind:             &pbsubstreams.Module_KindMap_{KindMap: &pbsubstreams.Module_KindMap{OutputType: "proto:sf.entity.v1.changes"}},
					BinaryIndex:      1,
					BinaryEntrypoint: "dbout_to_graphout",
					Inputs:           []*pbsubstreams.Module_Input{{Input: &pbsubstreams.Module_Input_Map_{Map: &pbsubstreams.Module_Input_Map{ModuleName: "example_dbout"}}}},
					Output:           &pbsubstreams.Module_Output{Type: "proto:sf.entity.v1.changes"},
				},
				{
					Name:             "example_dbout",
					Kind:             &pbsubstreams.Module_KindMap_{KindMap: &pbsubstreams.Module_KindMap{OutputType: "proto:sf.database.v1.changes"}},
					BinaryIndex:      1,
					BinaryEntrypoint: "example_dbout",
					Inputs:           []*pbsubstreams.Module_Input{{Input: &pbsubstreams.Module_Input_Source_{Source: &pbsubstreams.Module_Input_Source{Type: "block"}}}},
					Output:           &pbsubstreams.Module_Output{Type: "proto:sf.database.v1.changes"},
				},
			},
		},

		{
			name: "input's output type not matching",
			pkg: &pbsubstreams.Package{
				ProtoFiles: nil,
				Version:    0,
				Modules: &pbsubstreams.Modules{Modules: []*pbsubstreams.Module{
					{
						Name:             "use_module",
						Kind:             nil,
						BinaryIndex:      0,
						BinaryEntrypoint: "use_module",
						Inputs:           []*pbsubstreams.Module_Input{{Input: &pbsubstreams.Module_Input_Map_{Map: &pbsubstreams.Module_Input_Map{ModuleName: "B"}}}},
						InitialBlock:     5,
					},
					{
						Name:             "B",
						Kind:             &pbsubstreams.Module_KindMap_{KindMap: &pbsubstreams.Module_KindMap{OutputType: "proto:sf.database.v1.changes"}},
						BinaryIndex:      0,
						BinaryEntrypoint: "B",
						Inputs:           []*pbsubstreams.Module_Input{{Input: &pbsubstreams.Module_Input_Source_{Source: &pbsubstreams.Module_Input_Source{Type: "clock"}}}},
						Output:           &pbsubstreams.Module_Output{Type: "proto:sf.kv.v1.changes"},
					},
					{
						Name:             "dbout_to_graphout",
						Kind:             &pbsubstreams.Module_KindMap_{KindMap: &pbsubstreams.Module_KindMap{OutputType: "proto:sf.entity.v1.changes"}},
						BinaryIndex:      1,
						BinaryEntrypoint: "dbout_to_graphout",
						Inputs:           []*pbsubstreams.Module_Input{{Input: &pbsubstreams.Module_Input_Map_{Map: &pbsubstreams.Module_Input_Map{ModuleName: "example_dbout"}}}},
						Output:           &pbsubstreams.Module_Output{Type: "proto:sf.entity.v1.changes"},
					},
					{
						Name:             "example_dbout",
						Kind:             &pbsubstreams.Module_KindMap_{KindMap: &pbsubstreams.Module_KindMap{OutputType: "proto:sf.database.v1.changes"}},
						BinaryIndex:      1,
						BinaryEntrypoint: "example_dbout",
						Inputs:           []*pbsubstreams.Module_Input{{Input: &pbsubstreams.Module_Input_Source_{Source: &pbsubstreams.Module_Input_Source{Type: "block"}}}},
						Output:           &pbsubstreams.Module_Output{Type: "proto:sf.database.v1.changes"},
					},
				},
				},
			},
			manifest: &Manifest{
				Modules: []*Module{
					{Name: "use_module", Kind: "map", Inputs: []*Input{{Source: "proto:sf.database.v1.changes"}}, Use: "dbout_to_graphout"},
					{Name: "B", Kind: "map", Inputs: []*Input{{Source: "clock"}}, Output: StreamOutput{Type: "proto:sf.database.v1.changes"}},
					{Name: "dbout_to_graphout", Kind: "map", Inputs: []*Input{{Source: "proto:sf.database.v1.changes"}}},
					{Name: "example_dbout", Kind: "map", Inputs: []*Input{{Source: "block"}}, Output: StreamOutput{Type: "proto:sf.database.v1.changes"}},
				},
			},
			expectedError: "checking inputs for module \"use_module\": module \"use_module\": input \"map:{module_name:\\\"B\\\"}\" has different output than the used module \"dbout_to_graphout\": input \"map:{module_name:\\\"example_dbout\\\"}\"",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := handleUseModules(c.pkg, c.manifest)
			if c.expectedError != "" {
				require.Error(t, err)
				require.Contains(t, err.Error(), c.expectedError)
				return
			}
			require.NoError(t, err)

			for index, mod := range c.pkg.Modules.Modules {
				require.Equal(t, mod.String(), c.expectedOutputModules[index].String())
			}
		})
	}
}
