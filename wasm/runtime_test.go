package wasm

import (
	"fmt"
	"io/ioutil"
	"testing"

	pbcodec "github.com/streamingfast/sparkle/pb/sf/ethereum/codec/v1"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
)

//go:generate ./build-examples.sh

func TestRustInstance(t *testing.T) {
	wasmCode, err := ioutil.ReadFile("./example-block/pkg/example_block_bg.wasm")
	require.NoError(t, err)

	mod, err := NewModule(wasmCode)
	require.NoError(t, err, "filename: example_block_bg.wasm")
	instance, err := mod.NewInstance("map")
	require.NoError(t, err, "new instance")

	block := &pbcodec.Block{
		Ver:    1,
		Number: 1234,
		Hash:   []byte{0x01, 0x02, 0x03, 0x04},
		Header: &pbcodec.BlockHeader{
			ParentHash: []byte{0x00, 0x01, 0x02, 0x03},
		},
		TransactionTraces: []*pbcodec.TransactionTrace{
			{Hash: []byte{0x03, 0x03, 0x03, 0x03}},
			{Hash: []byte{0x04, 0x04, 0x04, 0x04}},
		},
	}
	blockBytes, err := proto.Marshal(block)
	require.NoError(t, err)

	err = instance.Execute([]*Input{{Name: "block", Type: InputStream, StreamData: blockBytes}})
	if err != nil {
		fmt.Printf("error here: %T, %v\n", err, err)
	}
	require.NoError(t, err)

	_, err = proto.Marshal(block.Header)
	require.NoError(t, err)
}
