package pipeline

import (
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
	"github.com/streamingfast/substreams/state"
	"github.com/stretchr/testify/require"
	"testing"
)

var reversibleOutputs = map[uint64][]*pbsubstreams.ModuleOutput{
	10: {
		{Name: "module_1"}, {Name: "module_2"}, {Name: "module_3"},
	},
	20: {
		{Name: "module_1"}, {Name: "module_2"}, {Name: "module_3"},
	},
	30: {
		{Name: "module_1"}, {Name: "module_2"}, {Name: "module_3"},
	},
	40: {
		{Name: "module_1"}, {Name: "module_2"}, {Name: "module_3"},
	},
	50: {
		{Name: "module_1"}, {Name: "module_2"}, {Name: "module_3"},
	},
}

var reversibleModules = map[string][]*pbsubstreams.Module{
	"10": {
		{Name: "module_1"}, {Name: "module_2"}, {Name: "module_3"},
	},
	"20": {
		{Name: "module_1"}, {Name: "module_2"}, {Name: "module_3"},
	},
	"30": {
		{Name: "module_1"}, {Name: "module_2"}, {Name: "module_3"},
	},
	"40": {
		{Name: "module_1"}, {Name: "module_2"}, {Name: "module_3"},
	},
	"50": {
		{Name: "module_1"}, {Name: "module_2"}, {Name: "module_3"},
	},
}

func Test_HandleIrreversibility(t *testing.T) {

	tests := []struct {
		name              string
		reversibleOutputs map[string][]*pbsubstreams.Module
		blockNumbers      []uint64
		expectedOutputs   map[uint64][]*pbsubstreams.ModuleOutput
	}{
		{
			name:              "handle irreversibility for block 20",
			reversibleOutputs: reversibleModules,
			blockNumbers:      []uint64{20},
			expectedOutputs: map[uint64][]*pbsubstreams.ModuleOutput{
				10: {
					{Name: "module_1"}, {Name: "module_2"}, {Name: "module_3"},
				},
				30: {
					{Name: "module_1"}, {Name: "module_2"}, {Name: "module_3"},
				},
				40: {
					{Name: "module_1"}, {Name: "module_2"}, {Name: "module_3"},
				},
				50: {
					{Name: "module_1"}, {Name: "module_2"}, {Name: "module_3"},
				},
			},
		},
		{
			name:              "handle irreversibility for block 20 and 30",
			reversibleOutputs: reversibleModules,
			blockNumbers:      []uint64{20, 30},
			expectedOutputs: map[uint64][]*pbsubstreams.ModuleOutput{
				10: {
					{Name: "module_1"}, {Name: "module_2"}, {Name: "module_3"},
				},
				40: {
					{Name: "module_1"}, {Name: "module_2"}, {Name: "module_3"},
				},
				50: {
					{Name: "module_1"}, {Name: "module_2"}, {Name: "module_3"},
				},
			},
		},
		{
			name:              "handle irreversibility for block 20, 30, 40 and 50",
			reversibleOutputs: reversibleModules,
			blockNumbers:      []uint64{20, 30, 40, 50},
			expectedOutputs: map[uint64][]*pbsubstreams.ModuleOutput{
				10: {
					{Name: "module_1"}, {Name: "module_2"}, {Name: "module_3"},
				},
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			forkHandler := &ForkHandler{
				reversibleOutputs: reversibleOutputs,
			}
			for _, blockNum := range test.blockNumbers {
				forkHandler.handleIrreversible(blockNum)
			}
			require.Equal(t, test.expectedOutputs, forkHandler.reversibleOutputs)
		})
	}
}

func Test_ReverseDeltas(t *testing.T) {
	testCases := []struct {
		name        string
		storeMap    map[string]*state.Store
		deltaGetter *TestStoreDeltas
		expectedKV  map[string][]byte
	}{
		{
			name: "reverse one delta",
			storeMap: map[string]*state.Store{
				"module_1": {
					Name: "module_1",
					Deltas: []*pbsubstreams.StoreDelta{
						{
							Operation: pbsubstreams.StoreDelta_CREATE,
							Key:       "key_1",
							NewValue:  []byte{99},
						},
					},
					KV: map[string][]byte{
						"key_1": {99},
					},
				},
			},
			deltaGetter: &TestStoreDeltas{
				deltas: []*pbsubstreams.StoreDelta{
					{
						Operation: pbsubstreams.StoreDelta_CREATE,
						Key:       "key_1",
						NewValue:  []byte{99},
					},
				},
			},
			expectedKV: map[string][]byte{},
		},
		{
			name: "reverse a delta when multiple deltas were applied",
			storeMap: map[string]*state.Store{
				"module_1": {
					Name: "module_1",
					Deltas: []*pbsubstreams.StoreDelta{
						{
							Operation: pbsubstreams.StoreDelta_CREATE,
							Key:       "key_1",
							NewValue:  []byte{99},
						},
						{
							Operation: pbsubstreams.StoreDelta_UPDATE,
							Key:       "key_1",
							OldValue:  []byte{99},
							NewValue:  []byte{100},
						},
					},
					KV: map[string][]byte{
						"key_1": {100},
					},
				},
			},
			deltaGetter: &TestStoreDeltas{
				deltas: []*pbsubstreams.StoreDelta{
					{
						Operation: pbsubstreams.StoreDelta_UPDATE,
						Key:       "key_1",
						OldValue:  []byte{99},
						NewValue:  []byte{100},
					},
				},
			},
			expectedKV: map[string][]byte{
				"key_1": {99},
			},
		},
		{
			name: "reverse multiple deltas",
			storeMap: map[string]*state.Store{
				"module_1": {
					Name: "module_1",
					Deltas: []*pbsubstreams.StoreDelta{
						{
							Operation: pbsubstreams.StoreDelta_CREATE,
							Key:       "key_1",
							NewValue:  []byte{99},
						},
						{
							Operation: pbsubstreams.StoreDelta_CREATE,
							Key:       "key_2",
							NewValue:  []byte{100},
						},
						{
							Operation: pbsubstreams.StoreDelta_UPDATE,
							Key:       "key_1",
							OldValue:  []byte{99},
							NewValue:  []byte{100},
						},
						{
							Operation: pbsubstreams.StoreDelta_DELETE,
							Key:       "key_1",
							OldValue:  []byte{100},
						},
						{
							Operation: pbsubstreams.StoreDelta_UPDATE,
							Key:       "key_2",
							OldValue:  []byte{100},
							NewValue:  []byte{150},
						},
					},
					KV: map[string][]byte{
						"key_2": {150},
					},
				},
			},
			deltaGetter: &TestStoreDeltas{
				deltas: []*pbsubstreams.StoreDelta{
					{
						Operation: pbsubstreams.StoreDelta_DELETE,
						Key:       "key_1",
						OldValue:  []byte{100},
					},
					{
						Operation: pbsubstreams.StoreDelta_UPDATE,
						Key:       "key_2",
						OldValue:  []byte{100},
						NewValue:  []byte{150},
					},
				},
			},
			expectedKV: map[string][]byte{
				"key_1": {100},
				"key_2": {100},
			},
		},
	}

	for _, test := range testCases {
		t.Run(test.name, func(t *testing.T) {
			for _, module := range test.storeMap {
				reverseDeltas(test.storeMap, module.Name, test.deltaGetter)
			}
			require.Equal(t, test.expectedKV, test.storeMap["module_1"].KV)
		})
	}
}

type TestStoreDeltas struct {
	deltas []*pbsubstreams.StoreDelta
}

func (t *TestStoreDeltas) GetDeltas() []*pbsubstreams.StoreDelta {
	return t.deltas
}
