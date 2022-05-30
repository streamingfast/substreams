package tui

import "sort"

func mergeRangeLists(prevRanges ranges, newRange *blockRange) ranges {
	// fmt.Println("BOO", prevRanges, newRange)
	var stretched bool
	for _, prevRange := range prevRanges {
		if newRange.Start <= prevRange.End {
			if prevRange.End < newRange.End {
				prevRange.End = newRange.End
				stretched = true
				break
			}
		} else if newRange.End >= prevRange.Start {
			if prevRange.Start > newRange.Start {
				prevRange.Start = newRange.Start
				stretched = true
				break
			}
		}
	}
	if !stretched {
		prevRanges = append(prevRanges, newRange)
	}
	// _ = stretched
	// prevRanges = append(prevRanges, newRange)
	sort.Sort(prevRanges)
	return prevRanges
	//return reduceOverlaps(prevRanges)
}

func reduceOverlaps(r ranges) ranges {
	if len(r) <= 1 {
		return r
	}

	var newRanges ranges
	for i := 0; i < len(r)-1; i++ {
		r1 := r[i]
		r2 := r[i+1]
		if r1.End >= r2.Start {
			// TODO: this would need to be recursive.. won't work otherwise
			newRanges = append(newRanges, &blockRange{Start: r1.Start, End: r2.End})
		} else {
			newRanges = append(newRanges, r1)
			if i == len(r) {
				newRanges = append(newRanges, r2)
			}
		}
	}
	return newRanges
}
