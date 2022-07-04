package pipeline

import (
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
	"testing"
)

func Test_ForkHandler(t *testing.T) {
	tests := []struct {
		name              string
		reversibleOutputs map[string][]*pbsubstreams.Module
		blockNumber       uint64
	}{
		{
			name: "delete outputs",
			reversibleOutputs: map[string][]*pbsubstreams.Module{
				"10": {
					{
						Name: "module_1",
					},
					{
						Name: "module_2",
					},
					{
						Name: "module_3",
					},
				},
				"20": {
					{
						Name: "module_1",
					},
					{
						Name: "module_2",
					},
					{
						Name: "module_3",
					},
				},
			},
			blockNumber: 10,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			f := &ForkHandler{
				reversibleOutputs: test.reversibleOutputs,
			}
		})
	}
}
