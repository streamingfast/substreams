package block

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/streamingfast/bstream"
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
	StartBlock        uint64
	ExclusiveEndBlock uint64
}

func NewRange(startBlock, exclusiveEndBlock uint64) *Range {
	if exclusiveEndBlock <= startBlock {
		panic(fmt.Sprintf("invalid block range start %d, end %d", startBlock, exclusiveEndBlock))
	}
	return &Range{startBlock, exclusiveEndBlock}
}

func (r *Range) String() string {
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

func (r *Range) ContainsBlockRef(blockRef bstream.BlockRef) bool {
	return r.Contains(blockRef.Num())
}

func (r *Range) Contains(blockNum uint64) bool {
	return blockNum >= r.StartBlock && blockNum < r.ExclusiveEndBlock
}

func (r *Range) Next(size uint64) *Range {
	return &Range{
		StartBlock:        r.ExclusiveEndBlock,
		ExclusiveEndBlock: r.ExclusiveEndBlock + size,
	}
}

func (r *Range) Previous(size uint64) *Range {
	return &Range{
		StartBlock:        r.StartBlock - size,
		ExclusiveEndBlock: r.StartBlock,
	}
}

func (r *Range) IsNext(next *Range, size uint64) bool {
	return r.Next(size).Equals(next)
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

func ParseRanges(in string) (out Ranges) {
	for _, e := range strings.Split(in, ",") {
		newRange := ParseRange(strings.Trim(e, " "))
		if newRange != nil {
			out = append(out, newRange)
		}
	}
	return
}

type Ranges []*Range

func (r Ranges) String() string {
	var rs []string
	for _, i := range r {
		rs = append(rs, i.String())
	}
	return strings.Join(rs, ",")
}

func (r Ranges) Len() int {
	return len(r)
}

func (r Ranges) Less(i, j int) bool {
	return r[i].StartBlock < r[j].StartBlock
}

func (r Ranges) Swap(i, j int) {
	r[i], r[j] = r[j], r[i]
}

func (r Ranges) Merged() (out Ranges) {
	for i := 0; i < len(r); i++ {
		curRange := r[i]
		if i == len(r)-1 {
			out = append(out, curRange)
			break
		}
		nextRange := r[i+1]
		if curRange.ExclusiveEndBlock != nextRange.StartBlock {
			out = append(out, curRange)
			continue
		}

		i++

		// Loop to squash all the next ones and create a new Range
		// from `curRange` and the latest matching `nextRange`.
		for j := i + 1; j < len(r); j++ {
			nextNextRange := r[j]
			if nextRange.ExclusiveEndBlock != nextNextRange.StartBlock {
				break
			}
			i++
			nextRange = nextNextRange
		}
		out = append(out, NewRange(curRange.StartBlock, nextRange.ExclusiveEndBlock))
	}
	return out
}

func (r Ranges) MergedBuckets(maxBucketSize uint64) (out Ranges) {
	for i := 0; i < len(r); i++ {
		currentRange := r[i]
		isLast := i == len(r)-1
		if isLast {
			out = append(out, currentRange)
			break
		}

		if currentRange.Size() > maxBucketSize {
			out = append(out, currentRange)
			continue
		}

		nextRange := r[i+1]
		if currentRange.ExclusiveEndBlock != nextRange.StartBlock || nextRange.ExclusiveEndBlock-currentRange.StartBlock > maxBucketSize {
			out = append(out, currentRange)
			continue
		}

		i++

		// Loop to squash all the next ones and create a new Range
		// from `currentRange` and the latest matching `nextRange`.
		for j := i + 1; j < len(r); j++ {
			nextNextRange := r[j]
			if nextRange.ExclusiveEndBlock != nextNextRange.StartBlock || nextNextRange.ExclusiveEndBlock-currentRange.StartBlock > maxBucketSize {
				break
			}
			i++
			nextRange = nextNextRange
		}
		out = append(out, NewRange(currentRange.StartBlock, nextRange.ExclusiveEndBlock))
	}
	return out
}
