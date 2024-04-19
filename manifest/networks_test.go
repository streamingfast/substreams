package manifest

import (
	"testing"

	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
	"github.com/stretchr/testify/assert"
)

func TestMergeNetwork(t *testing.T) {

	tests := []struct {
		name              string
		srcNetwork        *pbsubstreams.NetworkParams
		destNetwork       *pbsubstreams.NetworkParams
		expectDestNetwork *pbsubstreams.NetworkParams
		expectPanic       bool
	}{
		{
			name:              "some-nil",
			srcNetwork:        &pbsubstreams.NetworkParams{},
			destNetwork:       nil,
			expectDestNetwork: nil,
			expectPanic:       true,
		},
		{
			name:       "nil-some",
			srcNetwork: nil,
			destNetwork: &pbsubstreams.NetworkParams{
				InitialBlocks: map[string]uint64{
					"mod1": 10,
				},
				Params: map[string]string{
					"mod1": "mod=1",
				},
			},
			expectDestNetwork: &pbsubstreams.NetworkParams{
				InitialBlocks: map[string]uint64{
					"mod1": 10,
				},
				Params: map[string]string{
					"mod1": "mod=1",
				},
			},
		},

		{
			name: "just append",
			srcNetwork: &pbsubstreams.NetworkParams{
				InitialBlocks: map[string]uint64{
					"mod1": 10,
					"mod2": 20,
				},
			},
			destNetwork: &pbsubstreams.NetworkParams{
				InitialBlocks: map[string]uint64{
					"mod2": 22,
					"mod3": 33,
				},
			},
			expectDestNetwork: &pbsubstreams.NetworkParams{
				InitialBlocks: map[string]uint64{
					"src:mod1": 10,
					"src:mod2": 20,
					"mod2":     22,
					"mod3":     33,
				},
			},
		},
		{
			name: "overwrite mod2",
			srcNetwork: &pbsubstreams.NetworkParams{
				InitialBlocks: map[string]uint64{
					"mod1": 10,
					"mod2": 20,
				},
				Params: map[string]string{
					"mod2": "mod=2",
				},
			},
			destNetwork: &pbsubstreams.NetworkParams{
				InitialBlocks: map[string]uint64{
					"src:mod2": 22,
					"mod3":     33,
				},
				Params: map[string]string{
					"mod2": "mod=22",
				},
			},
			expectDestNetwork: &pbsubstreams.NetworkParams{
				InitialBlocks: map[string]uint64{
					"src:mod1": 10,
					"src:mod2": 22,
					"mod3":     33,
				},
				Params: map[string]string{
					"src:mod2": "mod=2",
					"mod2":     "mod=22",
				},
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if test.expectPanic {
				defer func() {
					if r := recover(); r == nil {
						t.Errorf("The code did not panic")
					}
				}()
			}
			mergeNetwork(test.srcNetwork, test.destNetwork, "src")
			assert.Equal(t, test.expectDestNetwork, test.destNetwork)

		})
	}

}

func TestMergeNetworks(t *testing.T) {

	var sepolia = &pbsubstreams.NetworkParams{
		InitialBlocks: map[string]uint64{
			"mod1": 10,
		},
		Params: map[string]string{
			"mod2": "addr=0xdeadbeef",
		},
	}
	var mainnet = &pbsubstreams.NetworkParams{
		InitialBlocks: map[string]uint64{
			"mod1": 20,
		},
		Params: map[string]string{
			"mod2": "addr=0x12121212",
		},
	}
	var sepoliaPrefixed = &pbsubstreams.NetworkParams{
		InitialBlocks: map[string]uint64{
			"src:mod1": 10,
		},
		Params: map[string]string{
			"src:mod2": "addr=0xdeadbeef",
		},
	}
	var mainnetPrefixed = &pbsubstreams.NetworkParams{
		InitialBlocks: map[string]uint64{
			"src:mod1": 20,
		},
		Params: map[string]string{
			"src:mod2": "addr=0x12121212",
		},
	}

	tests := []struct {
		name               string
		srcNetworks        map[string]*pbsubstreams.NetworkParams
		destNetworks       map[string]*pbsubstreams.NetworkParams
		expectError        string
		expectDestNetworks map[string]*pbsubstreams.NetworkParams
	}{
		{
			// src []  dest []  -> []
			name:               "nil-nil",
			srcNetworks:        nil,
			destNetworks:       nil,
			expectDestNetworks: nil,
		},
		{
			// case: src []  dest [mainnet,sepolia] -> [mainnet,sepolia]
			name:        "nil+some=some",
			srcNetworks: nil,
			destNetworks: map[string]*pbsubstreams.NetworkParams{
				"sepolia": sepolia,
				"mainnet": mainnet,
			},
			expectDestNetworks: map[string]*pbsubstreams.NetworkParams{
				"sepolia": sepolia,
				"mainnet": mainnet,
			},
		},
		{
			// case: src [mainnet,sepolia] dest [] -> [mainnet,sepolia]
			name: "some+nil=prefixed",
			srcNetworks: map[string]*pbsubstreams.NetworkParams{
				"sepolia": sepolia,
				"mainnet": mainnet,
			},
			destNetworks: nil,
			expectDestNetworks: map[string]*pbsubstreams.NetworkParams{
				"sepolia": sepoliaPrefixed,
				"mainnet": mainnetPrefixed,
			},
		},
		{
			// case: src [mainnet,sepolia] dest [mainnet,sepolia] -> [mainnet,sepolia]
			name: "same+same=same",
			srcNetworks: map[string]*pbsubstreams.NetworkParams{
				"mainnet": {},
				"sepolia": {},
			},
			destNetworks: map[string]*pbsubstreams.NetworkParams{
				"mainnet": mainnet,
				"sepolia": sepolia,
			},
			expectDestNetworks: map[string]*pbsubstreams.NetworkParams{
				"mainnet": mainnet,
				"sepolia": sepolia,
			},
		},

		{
			// case: src [mainnet] dest [sepolia] -> [mainnetPrefixed, sepolia]
			name: "dest-is-different",
			srcNetworks: map[string]*pbsubstreams.NetworkParams{
				"mainnet": mainnet,
			},
			destNetworks: map[string]*pbsubstreams.NetworkParams{
				"sepolia": sepolia,
			},
			expectDestNetworks: map[string]*pbsubstreams.NetworkParams{
				"mainnet": mainnetPrefixed,
				"sepolia": sepolia,
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			src := &pbsubstreams.Package{
				PackageMeta: []*pbsubstreams.PackageMetadata{
					{
						Name: "source_package",
					},
				},
				Networks: test.srcNetworks,
			}
			dest := &pbsubstreams.Package{
				PackageMeta: []*pbsubstreams.PackageMetadata{
					{
						Name: "dest_package",
					},
				},
				Networks: test.destNetworks,
			}
			mergeNetworks(src, dest, "src")
			assert.Equal(t, test.expectDestNetworks, dest.Networks)
		})
	}
}

func Test_validateNetworks(t *testing.T) {
	tests := []struct {
		name                   string
		pkg                    *pbsubstreams.Package
		includeImportedModules map[string]bool
		overrideNetwork        string
		wantErr                bool
	}{
		{
			name: "valid",
			pkg: &pbsubstreams.Package{
				Networks: map[string]*pbsubstreams.NetworkParams{
					"mainnet": {
						InitialBlocks: map[string]uint64{
							"mod1": 10,
						},
						Params: map[string]string{
							"mod1": "mod=1",
						},
					},
					"testnet": {
						InitialBlocks: map[string]uint64{
							"mod1": 20,
						},
						Params: map[string]string{
							"mod1": "mod=2",
						},
					},
				},
			},
		},
		{
			name: "invalid",
			pkg: &pbsubstreams.Package{
				Networks: map[string]*pbsubstreams.NetworkParams{
					"mainnet": {
						InitialBlocks: map[string]uint64{
							"mod1": 10,
						},
						Params: map[string]string{
							"mod1": "mod=1",
						},
					},
					"testnet": {
						InitialBlocks: map[string]uint64{
							"mod2": 20,
						},
						Params: map[string]string{
							"mod3": "mod=3",
						},
					},
				},
			},
			wantErr: true,
		},
		{
			name: "valid for modules declared by dependencies",
			pkg: &pbsubstreams.Package{
				Networks: map[string]*pbsubstreams.NetworkParams{
					"mainnet": {
						InitialBlocks: map[string]uint64{
							"mod1": 10,
						},
						Params: map[string]string{
							"mod1":     "mod=1",
							"lib:mod3": "mod=3",
						},
					},
					"testnet": {
						InitialBlocks: map[string]uint64{
							"mod1": 20,
						},
						Params: map[string]string{
							"mod1": "mod=11",
						},
					},
				},
			},
		},
		{
			name: "invalid for modules declared by dependencies when those modules are in the map of modules to include",
			pkg: &pbsubstreams.Package{
				Networks: map[string]*pbsubstreams.NetworkParams{
					"mainnet": {
						InitialBlocks: map[string]uint64{
							"mod1": 10,
						},
						Params: map[string]string{
							"mod1":     "mod=1",
							"lib:mod3": "mod=3",
						},
					},
					"testnet": {
						InitialBlocks: map[string]uint64{
							"mod1": 20,
						},
						Params: map[string]string{
							"mod1": "mod=11",
						},
					},
				},
			},
			includeImportedModules: map[string]bool{
				"lib:mod3": true,
			},
			wantErr: true,
		},

		{
			name: "valid for whole networks declared by dependencies",
			pkg: &pbsubstreams.Package{
				Networks: map[string]*pbsubstreams.NetworkParams{
					"mainnet": {
						InitialBlocks: map[string]uint64{
							"mod1":     10,
							"lib:mod2": 10,
						},
						Params: map[string]string{
							"mod1":     "mod=1",
							"lib:mod3": "mod=3",
						},
					},
					"testnet": {
						InitialBlocks: map[string]uint64{
							"lib:mod2": 20,
						},
						Params: map[string]string{
							"lib:mod3": "mod=3",
						},
					},
				},
			},
		},
		{
			// even if we depend on an imported module, we don't have to support all its networks
			name: "still valid for whole networks declared by dependencies even if they are in the list of modules to include",
			pkg: &pbsubstreams.Package{
				Networks: map[string]*pbsubstreams.NetworkParams{
					"mainnet": {
						InitialBlocks: map[string]uint64{
							"mod1":     10,
							"lib:mod2": 10,
						},
						Params: map[string]string{
							"mod1":     "mod=1",
							"lib:mod3": "mod=3",
						},
					},
					"testnet": {
						InitialBlocks: map[string]uint64{
							"lib:mod2": 20,
						},
						Params: map[string]string{
							"lib:mod3": "mod=3",
						},
					},
				},
			},
			includeImportedModules: map[string]bool{
				"lib:mod3": true,
			},
		},
		{
			name: "invalid for networks declared by dependencies that would be selected as default network",
			pkg: &pbsubstreams.Package{
				Network: "testnet",
				Networks: map[string]*pbsubstreams.NetworkParams{
					"mainnet": {
						InitialBlocks: map[string]uint64{
							"mod1": 10,
						},
						Params: map[string]string{
							"mod1": "mod=1",
						},
					},
					"testnet": {
						InitialBlocks: map[string]uint64{
							"lib:mod2": 20,
						},
						Params: map[string]string{
							"lib:mod3": "mod=3",
						},
					},
				},
			},
			wantErr: true,
		},
		{
			name: "network override works",
			pkg: &pbsubstreams.Package{
				Network: "testnet",
				Networks: map[string]*pbsubstreams.NetworkParams{
					"mainnet": {
						InitialBlocks: map[string]uint64{
							"mod1": 10,
						},
					},
				},
			},
			overrideNetwork: "mainnet",
		},
		{
			name: "invalid for empty networks declared by dependencies that would be selected as default network",
			pkg: &pbsubstreams.Package{
				Network: "unavailable",
				Networks: map[string]*pbsubstreams.NetworkParams{
					"mainnet": {
						InitialBlocks: map[string]uint64{
							"mod1": 10,
						},
						Params: map[string]string{
							"mod1": "mod=1",
						},
					},
				},
			},
			wantErr: true,
		},
		{
			name: "invalid for empty networks declared by dependencies that would be selected as default network",
			pkg: &pbsubstreams.Package{
				Network: "unavailable",
				Networks: map[string]*pbsubstreams.NetworkParams{
					"mainnet": {
						InitialBlocks: map[string]uint64{
							"mod1": 10,
						},
						Params: map[string]string{
							"mod1": "mod=1",
						},
					},
				},
			},
			wantErr: true,
		},
		{
			name: "valid for empty networks when a network is selected",
			pkg: &pbsubstreams.Package{
				Network:  "unavailable",
				Networks: nil,
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := validateNetworks(tt.pkg, tt.includeImportedModules, tt.overrideNetwork); (err != nil) != tt.wantErr {
				t.Errorf("validateNetworks() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
