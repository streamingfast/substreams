package block

import "strings"

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

func (r Ranges) Contains(input *Range) bool {
	for _, el := range r {
		if el.Equals(input) {
			return true
		}
	}
	return false
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

		if currentRange.Size() >= maxBucketSize-1 {
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
