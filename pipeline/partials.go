package pipeline

import (
	"bytes"
	"crypto/md5"
	"errors"
	"fmt"
	"hash/fnv"

	pbacme "github.com/streamingfast/dummy-blockchain/pb/sf/acme/type/v1"
	pbeth "github.com/streamingfast/firehose-ethereum/types/pb/sf/ethereum/type/v2"
	"github.com/streamingfast/substreams/storage/execout"
	"google.golang.org/protobuf/proto"
)

var errHashMismatch = errors.New("first transactions does not match expected hash, block is different")

func splitPartialblock(execOutput execout.ExecutionOutput, blockType string, prevTrxCount int, expectedHash []byte) (newCount int, newHash []byte, err error) {
	// TODO: this should be implemented in a "generic" fashion,
	// using Protobuf annotations + Protobuf reflection
	// (ex; the sf.ethereum.type.v2.Block could have an annotation on
	// the transactionTraces field marking it as "the" partial field)

	switch blockType {
	case "sf.acme.type.v1.Block":
		if blockData, _, err := execOutput.Get(blockType); err == nil {
			block := &pbacme.Block{}
			if err := proto.Unmarshal(blockData, block); err != nil {
				return 0, nil, fmt.Errorf("unmarshal block: %w", err)
			}

			var hashMismatch bool
			hasher := md5.New()
			for i, trx := range block.Transactions {
				if _, err := hasher.Write([]byte(trx.Hash)); err != nil {
					return 0, nil, fmt.Errorf("hash transaction: %w", err)
				}
				if i+1 == prevTrxCount {
					if !bytes.Equal(hasher.Sum(nil), expectedHash) {
						hashMismatch = true
					}
				}
			}
			newHash = hasher.Sum(nil)
			if hashMismatch || len(block.Transactions) < prevTrxCount {
				return len(block.Transactions), newHash, errHashMismatch
			}

			newCount = len(block.Transactions)
			block.Transactions = block.Transactions[prevTrxCount:]
			blockData, _ = proto.Marshal(block)
			if err := execOutput.Set(blockType, blockData); err != nil {
				return 0, nil, fmt.Errorf("set block: %w", err)
			}
		}
	case "sf.ethereum.type.v2.Block":
		if blockData, _, err := execOutput.Get(blockType); err == nil {
			block := &pbeth.Block{}
			if err := proto.Unmarshal(blockData, block); err != nil {
				return 0, nil, fmt.Errorf("unmarshal block: %w", err)
			}

			var hashMismatch bool
			// fnv64a is non-cryptographic but fast and allocation-free; we only
			// need to detect that the partial block matches the previous one, not
			// to defend against collisions. We hash the transaction hash plus a
			// deterministic marshalling of the receipt (and all its fields) for
			// each trace, in order.
			hasher := fnv.New64a()
			marshalOpts := proto.MarshalOptions{Deterministic: true}
			var receiptBuf []byte
			for i, trx := range block.TransactionTraces {
				hasher.Write(trx.Hash)
				if trx.Receipt != nil {
					receiptBuf, err = marshalOpts.MarshalAppend(receiptBuf[:0], trx.Receipt)
					if err != nil {
						return 0, nil, fmt.Errorf("hash transaction receipt: %w", err)
					}
					hasher.Write(receiptBuf)
				}
				if i+1 == prevTrxCount {
					if !bytes.Equal(hasher.Sum(nil), expectedHash) {
						hashMismatch = true
					}

				}
			}
			newHash = hasher.Sum(nil)
			if hashMismatch || len(block.TransactionTraces) < prevTrxCount {
				return len(block.TransactionTraces), newHash, errHashMismatch
			}

			newCount = len(block.TransactionTraces)
			block.TransactionTraces = block.TransactionTraces[prevTrxCount:]
			blockData, _ = proto.Marshal(block)
			if err := execOutput.Set(blockType, blockData); err != nil {
				return 0, nil, fmt.Errorf("set block: %w", err)
			}
		}
	default:
		return 0, nil, fmt.Errorf("unsupported block type %q for partial blocks", blockType)
	}

	return
}
