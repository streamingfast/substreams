package block

import (
	"fmt"
	"strconv"
	"strings"

	"go.uber.org/zap/zapcore"
)

func ParseRange(in string) *Range {
	if in == "" {
		return nil
	}
	ch := strings.Split(in, "-")
	lo, err := strconv.ParseInt(ch[0], 10, 64)
	if err != nil {
		panic(err)
	}
	hi, err := strconv.ParseInt(ch[1], 10, 64)
	if err != nil {
		panic(err)
	}
	return NewRange(uint64(lo), uint64(hi))
}

type Range struct {
	StartBlock        uint64 `json:"start_block"`
	ExclusiveEndBlock uint64 `json:"exclusive_end_block"`
}

func NewRange(startBlock, exclusiveEndBlock uint64) *Range {
	return &Range{startBlock, exclusiveEndBlock}
}

func (r *Range) String() string {
	if r == nil {
		return fmt.Sprintf("[nil)")
	}
	return fmt.Sprintf("[%d, %d)", r.StartBlock, r.ExclusiveEndBlock)
}

func (r *Range) MarshalLogObject(enc zapcore.ObjectEncoder) error {
	if r == nil {
		enc.AddBool("nil", true)
	} else {
		enc.AddUint64("start_block", r.StartBlock)
		enc.AddUint64("exclusive_end_block", r.ExclusiveEndBlock)
	}
	return nil
}

func (r *Range) IsEmpty() bool {
	return r.StartBlock == r.ExclusiveEndBlock
}

func (r *Range) Contains(blockNum uint64) bool {
	return blockNum >= r.StartBlock && blockNum < r.ExclusiveEndBlock
}

func (r *Range) IsAbove(blockNum uint64) bool {
	return blockNum > r.ExclusiveEndBlock
}

func (r *Range) IsBelow(blockNum uint64) bool {
	return blockNum < r.StartBlock
}

func (r *Range) IsOutOfBounds(blockNum uint64) bool {
	return !r.Contains(blockNum)
}

func (r *Range) Equals(other *Range) bool {
	return r.StartBlock == other.StartBlock && r.ExclusiveEndBlock == other.ExclusiveEndBlock
}

func (r *Range) Size() uint64 {
	return r.ExclusiveEndBlock - r.StartBlock
}

func (r *Range) Split(chunkSize uint64) []*Range {
	var res []*Range
	if r.ExclusiveEndBlock-r.StartBlock <= chunkSize {
		res = append(res, r)
		return res
	}

	currentEnd := (r.StartBlock + chunkSize) - (r.StartBlock+chunkSize)%chunkSize
	currentStart := r.StartBlock

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

func (r *Range) Len() uint64 {
	return r.ExclusiveEndBlock - r.StartBlock
}
