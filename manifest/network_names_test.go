package manifest

import (
	"testing"

	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
	"github.com/stretchr/testify/assert"
)

func TestNetworkNameWarnings(t *testing.T) {
	registry := fakeRegistry(map[string][]string{
		"mainnet":     {"mainnet"},
		"eth-mainnet": {"mainnet"},
		"sepolia":     {"sepolia"},
		"testnet":     {"sepolia", "holesky"},
	})

	tests := []struct {
		name     string
		pkg      *pbsubstreams.Package
		expected []string
	}{
		{
			name:     "known registry id",
			pkg:      &pbsubstreams.Package{Network: "mainnet"},
			expected: nil,
		},
		{
			name:     "alias resolving to a single network",
			pkg:      &pbsubstreams.Package{Network: "eth-mainnet"},
			expected: nil,
		},
		{
			name: "unknown name",
			pkg:  &pbsubstreams.Package{Network: "bogus-chain"},
			expected: []string{
				`network "bogus-chain" is not a known Firehose network registry ID or alias`,
			},
		},
		{
			name: "ambiguous alias",
			pkg:  &pbsubstreams.Package{Network: "testnet"},
			expected: []string{
				`network "testnet" is ambiguous, it resolves to: holesky, sepolia — use a specific registry ID`,
			},
		},
		{
			name: "every key of the networks map is checked",
			pkg: &pbsubstreams.Package{
				Network: "mainnet",
				Networks: map[string]*pbsubstreams.NetworkParams{
					"mainnet": {},
					"zzz-bad": {},
					"aaa-bad": {},
				},
			},
			expected: []string{
				`network "aaa-bad" is not a known Firehose network registry ID or alias`,
				`network "zzz-bad" is not a known Firehose network registry ID or alias`,
			},
		},
		{
			name: "default network is reported once even when it is also a map key",
			pkg: &pbsubstreams.Package{
				Network: "bogus-chain",
				Networks: map[string]*pbsubstreams.NetworkParams{
					"bogus-chain": {},
				},
			},
			expected: []string{
				`network "bogus-chain" is not a known Firehose network registry ID or alias`,
			},
		},
		{
			name:     "unset network is left to the dedicated warning",
			pkg:      &pbsubstreams.Package{},
			expected: nil,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assert.Equal(t, test.expected, networkNameWarnings(test.pkg, registry))
		})
	}
}

// fakeRegistry resolves a name to the registry IDs it matches, mimicking NetworkRegistry.FindAll.
func fakeRegistry(matches map[string][]string) func(string) []string {
	return func(name string) []string { return matches[name] }
}
