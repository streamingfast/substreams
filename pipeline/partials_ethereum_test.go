package pipeline

import (
	"bytes"
	"encoding/binary"
	"testing"

	"github.com/cespare/xxhash/v2"
	pbeth "github.com/streamingfast/firehose-ethereum/types/pb/sf/ethereum/type/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/encoding/protowire"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
)

// referenceSplitEthereumPartialBlock decodes the whole block and hashes it
// field by field; splitEthereumPartialBlock must give the same results.
func referenceSplitEthereumPartialBlock(t *testing.T, blockData []byte, prevTrxCount int, expectedHash []byte) (int, []byte, *pbeth.Block, error) {
	block := &pbeth.Block{}
	require.NoError(t, proto.Unmarshal(blockData, block))

	var hashMismatch bool
	hasher := xxhash.New()
	var scratch [8]byte
	writeUint64 := func(v uint64) {
		binary.LittleEndian.PutUint64(scratch[:], v)
		hasher.Write(scratch[:])
	}
	for i, trx := range block.TransactionTraces {
		hasher.Write(trx.Hash)
		if r := trx.Receipt; r != nil {
			hasher.Write(r.StateRoot)
			writeUint64(r.CumulativeGasUsed)
			for _, log := range r.Logs {
				hasher.Write(log.Address)
				for _, topic := range log.Topics {
					hasher.Write(topic)
				}
				hasher.Write(log.Data)
			}
			if r.BlobGasUsed != nil {
				writeUint64(*r.BlobGasUsed)
			}
			if r.BlobGasPrice != nil {
				hasher.Write(r.BlobGasPrice.Bytes)
			}
		}
		if i+1 == prevTrxCount && !bytes.Equal(hasher.Sum(nil), expectedHash) {
			hashMismatch = true
		}
	}
	newHash := hasher.Sum(nil)
	if hashMismatch || len(block.TransactionTraces) < prevTrxCount {
		return len(block.TransactionTraces), newHash, nil, errHashMismatch
	}
	newCount := len(block.TransactionTraces)
	block.TransactionTraces = block.TransactionTraces[prevTrxCount:]
	return newCount, newHash, block, nil
}

func assertSplitMatchesReference(t *testing.T, blockData []byte, prevTrxCount int, expectedHash []byte) {
	t.Helper()
	wantCount, wantHash, wantBlock, wantErr := referenceSplitEthereumPartialBlock(t, blockData, prevTrxCount, expectedHash)
	gotCount, gotHash, gotData, gotErr := splitEthereumPartialBlock(blockData, prevTrxCount, expectedHash)

	assert.Equal(t, wantErr, gotErr)
	assert.Equal(t, wantCount, gotCount)
	assert.Equal(t, wantHash, gotHash)
	if wantErr != nil {
		return
	}
	gotBlock := &pbeth.Block{}
	require.NoError(t, proto.Unmarshal(gotData, gotBlock))
	assert.True(t, proto.Equal(wantBlock, gotBlock), "output block differs from reference")
}

func hashOfFirstTraces(t *testing.T, blockData []byte, n int) []byte {
	block := &pbeth.Block{}
	require.NoError(t, proto.Unmarshal(blockData, block))
	block.TransactionTraces = block.TransactionTraces[:n]
	truncated, err := proto.Marshal(block)
	require.NoError(t, err)
	_, hash, _, err := referenceSplitEthereumPartialBlock(t, truncated, 0, nil)
	require.NoError(t, err)
	return hash
}

func TestSplitEthereumPartialBlock_Fixtures(t *testing.T) {
	for _, path := range ethPartialFixtures {
		data, trxCount := loadEthFixture(t, path)
		t.Run(fixtureName(path), func(t *testing.T) {
			for _, n := range []int{0, 1, trxCount / 2, trxCount - 1, trxCount} {
				hash := hashOfFirstTraces(t, data, n)
				assertSplitMatchesReference(t, data, n, hash)
				assertSplitMatchesReference(t, data, n, []byte("wrong hash"))
			}
			assertSplitMatchesReference(t, data, trxCount+1, nil)
		})
	}
}

func TestSplitEthereumPartialBlock_EdgeCases(t *testing.T) {
	u64 := func(v uint64) *uint64 { return &v }
	traces := []*pbeth.TransactionTrace{
		{Hash: []byte{0x01}},
		{Hash: []byte{0x02}, Receipt: &pbeth.TransactionReceipt{}},
		{Hash: []byte{0x03}, Receipt: &pbeth.TransactionReceipt{
			StateRoot:         []byte{0xaa},
			CumulativeGasUsed: 21000,
			Logs: []*pbeth.Log{
				{Address: []byte{0x10}, Topics: [][]byte{{0x20}, {0x21}}, Data: []byte{0x30}},
				{},
			},
			BlobGasUsed:  u64(0),
			BlobGasPrice: &pbeth.BigInt{},
		}},
		{Hash: []byte{0x04}, Receipt: &pbeth.TransactionReceipt{
			BlobGasUsed:  u64(131072),
			BlobGasPrice: &pbeth.BigInt{Bytes: []byte{0x01, 0x00}},
		}},
	}
	block := &pbeth.Block{Hash: []byte{0xff}, Number: 42, TransactionTraces: traces}
	data, err := proto.Marshal(block)
	require.NoError(t, err)

	t.Run("receipt and log variants", func(t *testing.T) {
		for n := 0; n <= len(traces)+1; n++ {
			assertSplitMatchesReference(t, data, n, hashOfFirstTraces(t, data, min(n, len(traces))))
		}
	})

	t.Run("fields interleaved with traces", func(t *testing.T) {
		var interleaved []byte
		for _, trace := range traces {
			traceData, err := proto.Marshal(trace)
			require.NoError(t, err)
			interleaved = protowire.AppendTag(interleaved, ethBlockTransactionTracesField, protowire.BytesType)
			interleaved = protowire.AppendBytes(interleaved, traceData)
			interleaved = protowire.AppendTag(interleaved, 3, protowire.VarintType) // number
			interleaved = protowire.AppendVarint(interleaved, 42)
		}
		for n := 0; n <= len(traces); n++ {
			assertSplitMatchesReference(t, interleaved, n, hashOfFirstTraces(t, interleaved, n))
		}
	})

	t.Run("repeated receipt occurrences merge", func(t *testing.T) {
		first, err := proto.Marshal(&pbeth.TransactionReceipt{StateRoot: []byte{0x01}, Logs: []*pbeth.Log{{Data: []byte{0x01}}}})
		require.NoError(t, err)
		second, err := proto.Marshal(&pbeth.TransactionReceipt{CumulativeGasUsed: 7, Logs: []*pbeth.Log{{Data: []byte{0x02}}}})
		require.NoError(t, err)
		var trace []byte
		trace = protowire.AppendTag(trace, ethTraceReceiptField, protowire.BytesType)
		trace = protowire.AppendBytes(trace, first)
		trace = protowire.AppendTag(trace, ethTraceHashField, protowire.BytesType)
		trace = protowire.AppendBytes(trace, []byte{0x09})
		trace = protowire.AppendTag(trace, ethTraceReceiptField, protowire.BytesType)
		trace = protowire.AppendBytes(trace, second)
		var blk []byte
		blk = protowire.AppendTag(blk, ethBlockTransactionTracesField, protowire.BytesType)
		blk = protowire.AppendBytes(blk, trace)
		assertSplitMatchesReference(t, blk, 0, nil)
	})

	t.Run("truncated block is an error", func(t *testing.T) {
		_, _, _, err := splitEthereumPartialBlock(data[:len(data)-1], 0, nil)
		assert.Error(t, err)
	})
}

func TestEthereumFieldNumbersMatchSchema(t *testing.T) {
	block := (&pbeth.Block{}).ProtoReflect().Descriptor()
	trace := (&pbeth.TransactionTrace{}).ProtoReflect().Descriptor()
	receipt := (&pbeth.TransactionReceipt{}).ProtoReflect().Descriptor()
	log := (&pbeth.Log{}).ProtoReflect().Descriptor()
	bigInt := (&pbeth.BigInt{}).ProtoReflect().Descriptor()

	tests := []struct {
		message  protoreflect.MessageDescriptor
		name     protoreflect.Name
		number   protowire.Number
		kind     protoreflect.Kind
		repeated bool
		// element is the message type of a MessageKind field
		element protoreflect.MessageDescriptor
	}{
		{block, "transaction_traces", ethBlockTransactionTracesField, protoreflect.MessageKind, true, trace},
		{trace, "hash", ethTraceHashField, protoreflect.BytesKind, false, nil},
		{trace, "receipt", ethTraceReceiptField, protoreflect.MessageKind, false, receipt},
		{receipt, "state_root", ethReceiptStateRootField, protoreflect.BytesKind, false, nil},
		{receipt, "cumulative_gas_used", ethReceiptCumulativeGasUsedField, protoreflect.Uint64Kind, false, nil},
		{receipt, "logs", ethReceiptLogsField, protoreflect.MessageKind, true, log},
		{receipt, "blob_gas_used", ethReceiptBlobGasUsedField, protoreflect.Uint64Kind, false, nil},
		{receipt, "blob_gas_price", ethReceiptBlobGasPriceField, protoreflect.MessageKind, false, bigInt},
		{log, "address", ethLogAddressField, protoreflect.BytesKind, false, nil},
		{log, "topics", ethLogTopicsField, protoreflect.BytesKind, true, nil},
		{log, "data", ethLogDataField, protoreflect.BytesKind, false, nil},
		{bigInt, "bytes", ethBigIntBytesField, protoreflect.BytesKind, false, nil},
	}

	for _, test := range tests {
		t.Run(string(test.message.Name())+"."+string(test.name), func(t *testing.T) {
			field := test.message.Fields().ByName(test.name)
			require.NotNil(t, field, "field not found in schema")
			assert.Equal(t, test.number, field.Number())
			assert.Equal(t, test.kind, field.Kind())
			assert.Equal(t, test.repeated, field.IsList())
			if test.element != nil {
				assert.Equal(t, test.element.FullName(), field.Message().FullName())
			}
		})
	}
}
