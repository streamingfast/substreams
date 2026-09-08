package tools

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func intPtr(i int) *int {
	return &i
}

func TestParseEndpoint(t *testing.T) {
	tests := []struct {
		name     string
		endpoint string
		want     struct {
			host       string
			startBlock *int
			params     map[string]string
			err        error
		}
	}{
		{
			name:     "valid endpoint with params",
			endpoint: "my.domain:443?namespace=eth-mainnet",
			want: struct {
				host       string
				startBlock *int
				params     map[string]string
				err        error
			}{
				host: "my.domain:443",
				params: map[string]string{
					"namespace": "eth-mainnet",
				},
				startBlock: nil,
				err:        nil,
			},
		},
		{
			name:     "another example with both",
			endpoint: "arb-one.streamingfast.io:443@123?namespace=arb-one",
			want: struct {
				host       string
				startBlock *int
				params     map[string]string
				err        error
			}{
				host: "arb-one.streamingfast.io:443",
				params: map[string]string{
					"namespace": "arb-one",
				},
				startBlock: intPtr(123),
				err:        nil,
			},
		},
		{
			name:     "valid endpoint with params and start block",
			endpoint: "my.domain:443@-10?namespace=eth-mainnet&region=us-east",
			want: struct {
				host       string
				startBlock *int
				params     map[string]string
				err        error
			}{
				host: "my.domain:443",
				params: map[string]string{
					"namespace": "eth-mainnet",
					"region":    "us-east",
				},
				startBlock: intPtr(-10),
				err:        nil,
			},
		},
		{
			name:     "valid endpoint with reversed params and start block",
			endpoint: "my.domain:443?namespace=eth-mainnet&region=us-east@1234",
			want: struct {
				host       string
				startBlock *int
				params     map[string]string
				err        error
			}{
				host: "my.domain:443",
				params: map[string]string{
					"namespace": "eth-mainnet",
					"region":    "us-east",
				},
				startBlock: intPtr(1234),
				err:        nil,
			},
		},
		{
			name:     "valid endpoint no params",
			endpoint: "my.domain:443",
			want: struct {
				host       string
				startBlock *int
				params     map[string]string
				err        error
			}{
				host:       "my.domain:443",
				startBlock: nil,
				params:     nil,
				err:        nil,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			host, startBlock, params, err := parseEndpoint(tt.endpoint)
			require.Equal(t, tt.want.err, err)
			assert.EqualValues(t, tt.want.startBlock, startBlock)
			assert.Equal(t, tt.want.host, host)
			require.Equal(t, tt.want.params, params)
		})
	}
}

func TestExtractParams(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want map[string]string
	}{
		{
			name: "single param",
			in:   "namespace=eth-mainnet",
			want: map[string]string{
				"namespace": "eth-mainnet",
			},
		},
		{
			name: "multiple params",
			in:   "namespace=eth-mainnet&region=us-east-1",
			want: map[string]string{
				"namespace": "eth-mainnet",
				"region":    "us-east-1",
			},
		},
		{
			name: "no params",
			in:   "",
			want: map[string]string{},
		},
		{
			name: "empty param value",
			in:   "key=&blah=bloh",
			want: map[string]string{
				"key":  "",
				"blah": "bloh",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := extractParams(tt.in)
			require.NoError(t, err)
			if len(got) != len(tt.want) {
				t.Errorf("extractParams() got %v params, want %v", len(got), len(tt.want))
			}
			for k, v := range tt.want {
				if got[k] != v {
					t.Errorf("extractParams()[%q] = %v, want %v", k, got[k], v)
				}
			}
		})
	}
}

func TestTrackBlockAge(t *testing.T) {
	maxFreshness := 40 * time.Second
	poll := func(age time.Duration) *pollResult {
		return &pollResult{blockAge: &age, maxFreshness: &maxFreshness}
	}

	t.Run("flips only once the streak confirms it", func(t *testing.T) {
		state := &endpointState{}

		for range blockAgeFlipPolls - 1 {
			trackBlockAge("endpoint:443", state, poll(30*time.Second))
			assert.False(t, state.blockAgeAboveHalf, "flipped before the streak completed")
		}

		trackBlockAge("endpoint:443", state, poll(30*time.Second))
		assert.True(t, state.blockAgeAboveHalf)
	})

	t.Run("an age straddling the threshold never flips", func(t *testing.T) {
		state := &endpointState{}

		// This is the chain whose block interval sits near half of --max-freshness: without the
		// streak it would report a crossing on every single poll, forever, while healthy.
		for range 20 {
			trackBlockAge("endpoint:443", state, poll(25*time.Second))
			trackBlockAge("endpoint:443", state, poll(15*time.Second))
		}

		assert.False(t, state.blockAgeAboveHalf)
	})

	t.Run("no block age is not a crossing", func(t *testing.T) {
		state := &endpointState{blockAgeAboveHalf: true}

		for range blockAgeFlipPolls + 2 {
			trackBlockAge("endpoint:443", state, &pollResult{maxFreshness: &maxFreshness})
		}

		assert.True(t, state.blockAgeAboveHalf)
	})
}
