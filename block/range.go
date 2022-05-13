package block

import (
	"fmt"

	"github.com/streamingfast/bstream"
	"go.uber.org/zap/zapcore"
)

type Range struct {
	StartBlock        uint64
	ExclusiveEndBlock uint64
}

func (r *Range) String() string {
	return fmt.Sprintf("start: %d exclusiveEndBlock: %d", r.StartBlock, r.ExclusiveEndBlock)
}

func (r *Range) MarshalLogObject(enc zapcore.ObjectEncoder) error {
	enc.AddUint64("start_block", r.StartBlock)
	enc.AddUint64("end_block", r.ExclusiveEndBlock)

	return nil
}

func (r *Range) Contains(blockRef bstream.BlockRef) bool {
	return blockRef.Num() >= r.StartBlock && blockRef.Num() < r.ExclusiveEndBlock
}

func (r *Range) Split(chunkSize uint64) []*Range {
	var res []*Range
	if r.ExclusiveEndBlock-r.StartBlock <= chunkSize {
		res = append(res, r)
		return res
	}

	currentStart := r.StartBlock
	currentEnd := r.StartBlock + chunkSize

	for {
		res = append(res, &Range{
			StartBlock:        currentStart,
			ExclusiveEndBlock: currentEnd,
		})

		if currentEnd >= r.ExclusiveEndBlock {
			break
		}

		currentStart = currentEnd
		currentEnd = currentStart + chunkSize
		if currentEnd > r.ExclusiveEndBlock {
			currentEnd = r.ExclusiveEndBlock
		}
	}

	return res
}
