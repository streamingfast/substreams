package outputmodules

import pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"

func TestNew() *Graph {
	return &Graph{
		outputModule: &pbsubstreams.Module{
			Name: "",
		},
	}
}
