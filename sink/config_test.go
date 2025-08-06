package sink

import (
	"testing"

	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSinkerConfig_ExtractDefaultParams(t *testing.T) {
	tests := []struct {
		name           string
		pkg            *pbsubstreams.Package
		existingParams []string
		expected       []string
	}{
		{
			name: "no existing params, extracts defaults",
			pkg: &pbsubstreams.Package{
				Modules: &pbsubstreams.Modules{
					Modules: []*pbsubstreams.Module{
						{
							Name: "module1",
							Inputs: []*pbsubstreams.Module_Input{
								{
									Input: &pbsubstreams.Module_Input_Params_{
										Params: &pbsubstreams.Module_Input_Params{
											Value: "default_value_1",
										},
									},
								},
							},
						},
						{
							Name: "module2",
							Inputs: []*pbsubstreams.Module_Input{
								{
									Input: &pbsubstreams.Module_Input_Params_{
										Params: &pbsubstreams.Module_Input_Params{
											Value: "default_value_2",
										},
									},
								},
							},
						},
					},
				},
			},
			existingParams: []string{},
			expected:       []string{"module1=default_value_1", "module2=default_value_2"},
		},
		{
			name: "existing params override defaults",
			pkg: &pbsubstreams.Package{
				Modules: &pbsubstreams.Modules{
					Modules: []*pbsubstreams.Module{
						{
							Name: "module1",
							Inputs: []*pbsubstreams.Module_Input{
								{
									Input: &pbsubstreams.Module_Input_Params_{
										Params: &pbsubstreams.Module_Input_Params{
											Value: "default_value_1",
										},
									},
								},
							},
						},
						{
							Name: "module2",
							Inputs: []*pbsubstreams.Module_Input{
								{
									Input: &pbsubstreams.Module_Input_Params_{
										Params: &pbsubstreams.Module_Input_Params{
											Value: "default_value_2",
										},
									},
								},
							},
						},
					},
				},
			},
			existingParams: []string{"module1=custom_value"},
			expected:       []string{"module2=default_value_2"},
		},
		{
			name: "module without params input",
			pkg: &pbsubstreams.Package{
				Modules: &pbsubstreams.Modules{
					Modules: []*pbsubstreams.Module{
						{
							Name: "module1",
							Inputs: []*pbsubstreams.Module_Input{
								{
									Input: &pbsubstreams.Module_Input_Map_{
										Map: &pbsubstreams.Module_Input_Map{
											ModuleName: "some_other_module",
										},
									},
								},
							},
						},
						{
							Name: "module2",
							Inputs: []*pbsubstreams.Module_Input{
								{
									Input: &pbsubstreams.Module_Input_Params_{
										Params: &pbsubstreams.Module_Input_Params{
											Value: "default_value_2",
										},
									},
								},
							},
						},
					},
				},
			},
			existingParams: []string{},
			expected:       []string{"module2=default_value_2"},
		},
		{
			name:           "nil package",
			pkg:            nil,
			existingParams: []string{},
			expected:       []string{},
		},
		{
			name: "package with nil modules",
			pkg: &pbsubstreams.Package{
				Modules: nil,
			},
			existingParams: []string{},
			expected:       []string{},
		},
		{
			name: "empty existing params with empty values",
			pkg: &pbsubstreams.Package{
				Modules: &pbsubstreams.Modules{
					Modules: []*pbsubstreams.Module{
						{
							Name: "module1",
							Inputs: []*pbsubstreams.Module_Input{
								{
									Input: &pbsubstreams.Module_Input_Params_{
										Params: &pbsubstreams.Module_Input_Params{
											Value: "default_value_1",
										},
									},
								},
							},
						},
					},
				},
			},
			existingParams: []string{"", "module1=custom"},
			expected:       []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := &SinkerConfig{
				Pkg:    tt.pkg,
				Params: tt.existingParams,
			}

			result := config.ExtractDefaultParams()

			if tt.expected == nil {
				require.Nil(t, result)
			} else {
				assert.ElementsMatch(t, tt.expected, result)
			}
		})
	}
}
