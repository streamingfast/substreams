package exec

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/streamingfast/substreams/metrics"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
	"github.com/streamingfast/substreams/reqctx"
	"github.com/streamingfast/substreams/storage/execout"
	"github.com/streamingfast/substreams/storage/store"
	"github.com/streamingfast/substreams/wasm"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestGetWasmArgumentValues(t *testing.T) {
	testCases := []struct {
		name               string
		wasmArguments      []wasm.Argument
		expectedValues     map[string][]byte
		outputGetter       mockOutputGetter
		expectSingleParams bool
		expectedError      error
	}{
		{
			name:           "Empty wasm arguments",
			wasmArguments:  []wasm.Argument{},
			expectedValues: map[string][]byte{},
			expectedError:  nil,
		},
		{
			name: "single params value",
			wasmArguments: []wasm.Argument{
				wasm.NewParamsInput("test"),
			},
			expectedValues:     map[string][]byte{},
			expectSingleParams: true,
			expectedError:      nil,
		},
		{
			name: "Supported vs unused wasm arguments",
			wasmArguments: []wasm.Argument{
				wasm.NewParamsInput("test"),
				wasm.NewSourceInput("source", 0),
				wasm.NewMapInput("map", 0),
				wasm.NewStoreDeltaInput("storedelta", 0),
				&wasm.StoreReaderInput{},  // ignored
				&wasm.StoreWriterOutput{}, // ignored
			},
			expectedValues: map[string][]byte{
				"source":     nil,
				"map":        nil,
				"storedelta": nil,
			},
			expectedError: nil,
		},
		{
			name:          "Single wasm argument with non-nil value",
			wasmArguments: []wasm.Argument{wa("arg1")},
			outputGetter:  mockOutputGetter{"arg1": {1, 2, 3}},
			expectedValues: map[string][]byte{
				"arg1": {1, 2, 3},
			},
			expectedError: nil,
		},
		{
			name:          "Single wasm argument with no value",
			wasmArguments: []wasm.Argument{wa("arg1")},
			outputGetter:  mockOutputGetter{},
			expectedValues: map[string][]byte{
				"arg1": nil,
			},
			expectedError: nil,
		},
		{
			name: "Multiple wasm arguments with non-nil values",
			wasmArguments: []wasm.Argument{
				wa("arg1"),
				wa("arg2"),
				wa("arg3"),
			},
			outputGetter: mockOutputGetter{
				"arg1": {1, 2, 3},
				"arg2": {4, 5, 6},
				"arg3": {7, 8, 9},
			},
			expectedValues: map[string][]byte{
				"arg1": {1, 2, 3},
				"arg2": {4, 5, 6},
				"arg3": {7, 8, 9},
			},
			expectedError: nil,
		},
		{
			name: "Multiple wasm arguments with nil values",
			wasmArguments: []wasm.Argument{
				wa("arg1"),
				wa("arg2"),
				wa("arg3"),
			},
			expectedValues: map[string][]byte{
				"arg1": nil,
				"arg2": nil,
				"arg3": nil,
			},
			expectedError: nil,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			values, singleParams, err := getWasmArgumentValues(tc.wasmArguments, tc.outputGetter)
			if err != tc.expectedError {
				t.Errorf("Expected error: %v, but got: %v", tc.expectedError, err)
			}

			if len(values) != len(tc.expectedValues) {
				t.Errorf("Expected %d values, but got %d", len(tc.expectedValues), len(values))
			}

			if tc.expectSingleParams {
				assert.True(t, singleParams, "singleParams should be true")
			} else {
				assert.False(t, singleParams, "singleParams should be false")
			}

			for key, expectedValue := range tc.expectedValues {
				actualValue, ok := values[key]
				if !ok {
					t.Errorf("Expected value for key '%s' not found", key)
				}

				if !bytes.Equal(actualValue, expectedValue) {
					t.Errorf("Expected value for key '%s' to be %v, but got %v", key, expectedValue, actualValue)
				}
			}
		})
	}
}

func TestCanSkipExecution(t *testing.T) {
	testCases := []struct {
		name               string
		wasmArgumentValues map[string][]byte
		singleParams       bool
		expectedResult     bool
	}{
		{
			name: "All arguments have non-nil values",
			wasmArgumentValues: map[string][]byte{
				"arg1": {1, 2, 3},
				"arg2": {4, 5, 6},
				"arg3": {7, 8, 9},
			},
			expectedResult: false,
		},
		{
			name: "Some arguments have nil values",
			wasmArgumentValues: map[string][]byte{
				"arg1": {1, 2, 3},
				"arg2": nil,
				"arg3": {7, 8, 9},
			},
			expectedResult: false,
		},
		{
			name: "All arguments have nil values",
			wasmArgumentValues: map[string][]byte{
				"arg1": nil,
				"arg2": nil,
				"arg3": nil,
			},
			expectedResult: true,
		},
		{
			name:           "We have singleParams",
			singleParams:   true,
			expectedResult: false,
		},
		{
			name: "Some arguments have nil values, but a Clock exist",
			wasmArgumentValues: map[string][]byte{
				"arg1":                   nil,
				"arg2":                   nil,
				"sf.substreams.v1.Clock": {1, 2, 3},
			},
			expectedResult: true,
		},
		{
			name: "A single argument is a Clock",
			wasmArgumentValues: map[string][]byte{
				"sf.substreams.v1.Clock": {1, 2, 3},
			},
			expectedResult: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := canSkipExecution(tc.wasmArgumentValues, tc.singleParams)
			if result != tc.expectedResult {
				t.Errorf("Expected canSkipExecution to return %v, but got %v", tc.expectedResult, result)
			}
		})
	}
}

func TestWasmCallErrorClassification(t *testing.T) {
	testCases := []struct {
		name                string
		err                 error
		expectDeterministic bool
	}{
		{
			name:                "module instantiation failure",
			err:                 fmt.Errorf("%w: creating new instance: cannot create a memfd: Too many open files (os error 24)", wasm.ErrModuleInstantiation),
			expectDeterministic: false,
		},
		{
			name:                "store backend failure",
			err:                 fmt.Errorf("store backend failed to set key %q: %w", "key", store.ErrStoreBackendFailure),
			expectDeterministic: false,
		},
		{
			name:                "wasm execution failure",
			err:                 errors.New("calling entrypoint: unreachable"),
			expectDeterministic: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			stats := metrics.NewReqStats(&metrics.Config{}, nil, nil, zap.NewNop())
			executor := &BaseExecutor{
				ctx:           reqctx.WithReqStats(context.Background(), stats),
				moduleName:    "map_test",
				wasmModule:    &failingModule{err: tc.err},
				wasmArguments: []wasm.Argument{wa("arg1")},
				entrypoint:    "map_test",
			}
			outputGetter := clockOutputGetter{
				mockOutputGetter: mockOutputGetter{"arg1": {1, 2, 3}},
				clock:            &pbsubstreams.Clock{Id: "block-1", Number: 1},
			}

			_, err := executor.wasmCall(outputGetter, false, nil, nil)
			require.ErrorContains(t, err, tc.err.Error())
			assert.Equal(t, tc.expectDeterministic, errors.Is(err, wasm.ErrWasmDeterministicExec))
		})
	}
}

func wa(in string) *wasm.MapInput {
	return wasm.NewMapInput(in, 0)
}

type mockOutputGetter map[string][]byte

func (m mockOutputGetter) Len() int {
	return len(m)
}
func (m mockOutputGetter) Clock() *pbsubstreams.Clock {
	panic("not implemented")
}

func (m mockOutputGetter) Get(name string) ([]byte, bool, error) {
	if val, ok := m[name]; ok {
		return val, false, nil
	}
	return nil, false, execout.ErrNotFound
}

type clockOutputGetter struct {
	mockOutputGetter
	clock *pbsubstreams.Clock
}

func (c clockOutputGetter) Clock() *pbsubstreams.Clock {
	return c.clock
}
