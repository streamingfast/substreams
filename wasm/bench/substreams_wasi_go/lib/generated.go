package lib

import (
	"fmt"

	"github.com/streamingfast/substreams/wasm/bench/substreams_wasi_go/pb"
	"github.com/streamingfast/substreams/wasm/wasi/substream"
)

func init() {
	substream.Register("mapBlock", ExecuteMapBlock)
}

type MapBlockInput struct {
	block      *pb.Block
	readStore1 substream.StoreGet[string]
	readStore2 substream.StoreGet[string]
}

func ExecuteMapBlock(input []byte) error {
	res := &pb.MapBlockInput{}
	err := res.UnmarshalVT(input)
	if err != nil {
		return fmt.Errorf("unmarshalling args: %w", err)
	}
	mapBlockInputs := &MapBlockInput{
		block:      res.Block,
		readStore1: substream.NewGetStringStore(res.ReadStore),
		readStore2: substream.NewGetStringStore(res.ReadStore2),
	}

	out, err := mapBlock(mapBlockInputs)
	if err != nil {
		return fmt.Errorf("mapping block: %w", err)
	}
	data, err := out.MarshalVT()
	if err != nil {
		return fmt.Errorf("marshalling output: %w", err)
	}

	_, err = substream.WriteOutput(data)
	if err != nil {
		return fmt.Errorf("writing output: %w", err)
	}

	return nil
}
