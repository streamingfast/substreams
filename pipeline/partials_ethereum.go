package pipeline

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"hash"

	"github.com/cespare/xxhash/v2"
	"google.golang.org/protobuf/encoding/protowire"
)

// Field numbers from sf.ethereum.type.v2 (firehose-ethereum/types) read by
// splitEthereumPartialBlock. They must stay in sync with that schema.
const (
	ethBlockTransactionTracesField = 10

	ethTraceHashField    = 21
	ethTraceReceiptField = 31

	ethReceiptStateRootField         = 1
	ethReceiptCumulativeGasUsedField = 2
	ethReceiptLogsField              = 4
	ethReceiptBlobGasUsedField       = 5
	ethReceiptBlobGasPriceField      = 6

	ethLogAddressField = 1
	ethLogTopicsField  = 2
	ethLogDataField    = 3

	ethBigIntBytesField = 1
)

// splitEthereumPartialBlock works on the encoded sf.ethereum.type.v2.Block
// without decoding it: it walks the wire format to hash the fields of each
// transaction trace, then drops the first prevTrxCount traces by copying the
// remaining bytes. Fields are read with protobuf semantics (last value wins for
// singular fields, repeated occurrences of a message merge) and unknown or
// mistyped fields are skipped, so the hash is the same as hashing the decoded
// message.
//
// Only the top-level fields, the traces, their receipt, logs and blob gas
// price are validated; other nested messages are copied as-is.
func splitEthereumPartialBlock(blockData []byte, prevTrxCount int, expectedHash []byte) (newCount int, newHash []byte, out []byte, err error) {
	h := &ethTraceHasher{hasher: xxhash.New()}

	var (
		hashMismatch bool
		trxCount     int
		skipStart    = -1 // offset of the first trace to drop
		skipEnd      int  // offset right after the last trace to drop
		keptInSkip   bool // a non-trace field sits between dropped traces
	)

	for pos := 0; pos < len(blockData); {
		num, typ, tagLen := protowire.ConsumeTag(blockData[pos:])
		if tagLen < 0 {
			return 0, nil, nil, fmt.Errorf("unmarshal block: %w", protowire.ParseError(tagLen))
		}
		valLen := protowire.ConsumeFieldValue(num, typ, blockData[pos+tagLen:])
		if valLen < 0 {
			return 0, nil, nil, fmt.Errorf("unmarshal block: %w", protowire.ParseError(valLen))
		}
		fieldEnd := pos + tagLen + valLen

		if num == ethBlockTransactionTracesField && typ == protowire.BytesType {
			trace, _ := protowire.ConsumeBytes(blockData[pos+tagLen:])
			if err := h.writeTrace(trace); err != nil {
				return 0, nil, nil, fmt.Errorf("unmarshal block: %w", err)
			}
			trxCount++

			if trxCount <= prevTrxCount {
				if skipStart == -1 {
					skipStart = pos
				} else if pos != skipEnd {
					keptInSkip = true
				}
				skipEnd = fieldEnd
			}
			if trxCount == prevTrxCount && !bytes.Equal(h.hasher.Sum(nil), expectedHash) {
				hashMismatch = true
			}
		}

		pos = fieldEnd
	}

	newHash = h.hasher.Sum(nil)
	if hashMismatch || trxCount < prevTrxCount {
		return trxCount, newHash, nil, errHashMismatch
	}

	if skipStart == -1 {
		return trxCount, newHash, blockData, nil
	}
	if !keptInSkip {
		out = make([]byte, 0, len(blockData)-(skipEnd-skipStart))
		out = append(out, blockData[:skipStart]...)
		out = append(out, blockData[skipEnd:]...)
		return trxCount, newHash, out, nil
	}
	return trxCount, newHash, dropLeadingTraces(blockData, prevTrxCount), nil
}

// dropLeadingTraces copies blockData without its first n transaction traces.
// It is the slow path for blocks where other fields are interleaved with the
// traces; blockData must already be validated.
func dropLeadingTraces(blockData []byte, n int) []byte {
	out := make([]byte, 0, len(blockData))
	dropped := 0
	for pos := 0; pos < len(blockData); {
		num, typ, tagLen := protowire.ConsumeTag(blockData[pos:])
		fieldEnd := pos + tagLen + protowire.ConsumeFieldValue(num, typ, blockData[pos+tagLen:])
		if num == ethBlockTransactionTracesField && typ == protowire.BytesType && dropped < n {
			dropped++
		} else {
			out = append(out, blockData[pos:fieldEnd]...)
		}
		pos = fieldEnd
	}
	return out
}

// ethTraceHasher writes, for each trace, the transaction hash and the receipt
// fields (and its logs) into hasher. Fixed-width integers double as separators
// between records. Its slices are reused across traces to avoid allocations.
type ethTraceHasher struct {
	hasher  hash.Hash64
	scratch [8]byte

	receipts [][]byte
	logs     [][]byte
	topics   [][]byte
}

func (h *ethTraceHasher) writeUint64(v uint64) {
	binary.LittleEndian.PutUint64(h.scratch[:], v)
	h.hasher.Write(h.scratch[:])
}

func (h *ethTraceHasher) writeTrace(trace []byte) error {
	var trxHash []byte
	h.receipts = h.receipts[:0]
	err := forEachField(trace, func(num protowire.Number, typ protowire.Type, v []byte) error {
		if typ != protowire.BytesType {
			return nil
		}
		switch num {
		case ethTraceHashField:
			trxHash = v
		case ethTraceReceiptField:
			h.receipts = append(h.receipts, v)
		}
		return nil
	})
	if err != nil {
		return err
	}

	h.hasher.Write(trxHash)
	if len(h.receipts) == 0 {
		return nil
	}

	var (
		stateRoot         []byte
		cumulativeGasUsed uint64
		blobGasUsed       uint64
		hasBlobGasUsed    bool
		blobGasPrice      []byte
		hasBlobGasPrice   bool
	)
	h.logs = h.logs[:0]
	for _, receipt := range h.receipts {
		err := forEachField(receipt, func(num protowire.Number, typ protowire.Type, v []byte) error {
			switch {
			case num == ethReceiptStateRootField && typ == protowire.BytesType:
				stateRoot = v
			case num == ethReceiptCumulativeGasUsedField && typ == protowire.VarintType:
				cumulativeGasUsed, _ = protowire.ConsumeVarint(v)
			case num == ethReceiptLogsField && typ == protowire.BytesType:
				h.logs = append(h.logs, v)
			case num == ethReceiptBlobGasUsedField && typ == protowire.VarintType:
				blobGasUsed, _ = protowire.ConsumeVarint(v)
				hasBlobGasUsed = true
			case num == ethReceiptBlobGasPriceField && typ == protowire.BytesType:
				hasBlobGasPrice = true
				return forEachField(v, func(num protowire.Number, typ protowire.Type, v []byte) error {
					if num == ethBigIntBytesField && typ == protowire.BytesType {
						blobGasPrice = v
					}
					return nil
				})
			}
			return nil
		})
		if err != nil {
			return err
		}
	}

	h.hasher.Write(stateRoot)
	h.writeUint64(cumulativeGasUsed)
	for _, log := range h.logs {
		if err := h.writeLog(log); err != nil {
			return err
		}
	}
	if hasBlobGasUsed {
		h.writeUint64(blobGasUsed)
	}
	if hasBlobGasPrice {
		h.hasher.Write(blobGasPrice)
	}
	return nil
}

func (h *ethTraceHasher) writeLog(log []byte) error {
	var address, data []byte
	h.topics = h.topics[:0]
	err := forEachField(log, func(num protowire.Number, typ protowire.Type, v []byte) error {
		if typ != protowire.BytesType {
			return nil
		}
		switch num {
		case ethLogAddressField:
			address = v
		case ethLogTopicsField:
			h.topics = append(h.topics, v)
		case ethLogDataField:
			data = v
		}
		return nil
	})
	if err != nil {
		return err
	}

	h.hasher.Write(address)
	for _, topic := range h.topics {
		h.hasher.Write(topic)
	}
	h.hasher.Write(data)
	return nil
}

// forEachField calls fn for every field of the encoded message msg. v is the
// field payload: the bytes of a length-delimited field, or the raw encoded value
// for the other wire types.
func forEachField(msg []byte, fn func(num protowire.Number, typ protowire.Type, v []byte) error) error {
	for len(msg) > 0 {
		num, typ, tagLen := protowire.ConsumeTag(msg)
		if tagLen < 0 {
			return protowire.ParseError(tagLen)
		}
		msg = msg[tagLen:]
		valLen := protowire.ConsumeFieldValue(num, typ, msg)
		if valLen < 0 {
			return protowire.ParseError(valLen)
		}
		v := msg[:valLen]
		if typ == protowire.BytesType {
			v, _ = protowire.ConsumeBytes(v)
		}
		if err := fn(num, typ, v); err != nil {
			return err
		}
		msg = msg[valLen:]
	}
	return nil
}
