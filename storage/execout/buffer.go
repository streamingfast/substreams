package execout

import (
	"fmt"

	pbbstream "github.com/streamingfast/bstream/pb/sf/bstream/v1"

	"google.golang.org/protobuf/proto"

	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
	"github.com/streamingfast/substreams/wasm"
)

// Buffer holds the values produced by modules and exchanged between them
// as a sort of buffer.
// Here are the types of exec outputs per module type:
//
//	               values         valuesForFileOutput
//	---------------------------------------------------
//	store:         deltas               kvops
//	mapper:         data              same data
//	index:          keys                 --
type Buffer struct {
	values              map[string][]byte
	valuesForFileOutput map[string][]byte

	isExecSkippedFromIndex map[string]bool
	clock                  *pbsubstreams.Clock
}

func (i *Buffer) Len() (out int) {
	for _, v := range i.values {
		out += len(v)
	}

	return
}

func NewBuffer(blockType string, block *pbbstream.Block, clock *pbsubstreams.Clock) (*Buffer, error) {
	values := make(map[string][]byte)

	clockBytes, err := proto.Marshal(clock)
	if err != nil {
		return nil, fmt.Errorf("marshalling clock %d %q: %w", clock.Number, clock.Id, err)
	}
	values[wasm.ClockType] = clockBytes

	if block != nil {
		values[blockType] = block.Payload.Value
	}

	return &Buffer{
		clock:                  clock,
		values:                 values,
		valuesForFileOutput:    make(map[string][]byte),
		isExecSkippedFromIndex: make(map[string]bool),
	}, nil
}

func (i *Buffer) Clock() *pbsubstreams.Clock {
	return i.clock
}

func (i *Buffer) Get(moduleName string) (value []byte, cached bool, err error) {
	val, found := i.values[moduleName]
	if !found {
		return nil, false, ErrNotFound
	}
	return val, true, nil
}

func (i *Buffer) Set(moduleName string, value []byte, isSkippedFromIndex bool) (err error) {
	if isSkippedFromIndex {
		i.isExecSkippedFromIndex[moduleName] = true
		return nil
	}

	i.values[moduleName] = value
	return nil
}

func (i *Buffer) SetFileOutput(moduleName string, value []byte, isSkippedFromIndex bool) (err error) {
	if isSkippedFromIndex {
		i.isExecSkippedFromIndex[moduleName] = true
		i.valuesForFileOutput[moduleName] = nil
		return nil
	}

	i.valuesForFileOutput[moduleName] = value
	return nil
}

func (i *Buffer) IsSkippedFromIndex(moduleName string) bool {
	return i.isExecSkippedFromIndex[moduleName]
}
