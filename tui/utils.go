package tui

import (
	"fmt"
	"sort"
	"strings"
)

type ranges []*blockRange

func (r ranges) Len() int           { return len(r) }
func (r ranges) Less(i, j int) bool { return r[i].Start < r[j].Start }
func (r ranges) Swap(i, j int)      { r[i], r[j] = r[j], r[i] }

func (r ranges) LoHi() (uint64, uint64) {
	if len(r) == 0 {
		return 0, 0
	}
	return r[0].Start, r[len(r)-1].End
}
func (r ranges) Lo() uint64 { a, _ := r.LoHi(); return a }
func (r ranges) Hi() uint64 { _, b := r.LoHi(); return b }

// Covered assumes block ranges have reduced overlaps/junctions.
func (r ranges) Covered(lo, hi uint64) bool {
	for _, blockRange := range r {
		if blockRange.Start <= lo && hi <= blockRange.End {
			return true
		}
	}
	return false
}

func (r ranges) String() string {
	var out []string
	for _, m := range r {
		out = append(out, fmt.Sprintf("%s", m.String()))
	}
	return strings.Join(out, ", ")
}

// Covered assumes block ranges have reduced overlaps/junctions.
func (r ranges) PartiallyCovered(lo, hi uint64) bool {
	for _, blockRange := range r {
		if r.Covered(lo, hi) {
			return true
		}
		if blockRange.Start <= lo && lo <= blockRange.End {
			return true
		}
		if blockRange.Start <= hi && hi <= blockRange.End {
			return true
		}
		if blockRange.Start >= lo && hi >= blockRange.End {
			return true
		}
	}
	return false
}

type blockRange struct {
	Start uint64
	End   uint64
}

func (b blockRange) String() string {
	return fmt.Sprintf("%d-%d", b.Start, b.End)
}

type updatedRanges map[string]ranges

// LoHi returns the lowest and highest of all modules. The global span,
// used to determine the width and the divider of each printable cell.
func (u updatedRanges) LoHi() (lo uint64, hi uint64) {
	var loset bool
	for _, v := range u {
		tlo, thi := v.LoHi()
		if thi > hi {
			hi = thi
		}
		if !loset {
			lo = tlo
			loset = true
		} else if tlo < lo {
			lo = tlo
		}
	}
	return
}

func (u updatedRanges) Lo() uint64 { a, _ := u.LoHi(); return a }
func (u updatedRanges) Hi() uint64 { _, b := u.LoHi(); return b }

type newRange map[string]blockRange

func mergeRangeLists(prevRanges ranges, newRange *blockRange) ranges {
	// fmt.Println("merge input, prevRanges:", prevRanges, "new range:", newRange)
	var stretched bool
	for _, prevRange := range prevRanges {
		if newRange.Start <= prevRange.End+1 {
			if prevRange.End < newRange.End {
				prevRange.End = newRange.End
				stretched = true
				break
			}
		} else if newRange.End+1 >= prevRange.Start {
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
	sort.Sort(prevRanges)
	return reduceOverlaps(prevRanges)
}

func reduceOverlaps(r ranges) ranges {
	// fmt.Println("reduce input:", r)

	if len(r) <= 1 {
		return r
	}

	var newRanges ranges
	for i := 0; i < len(r)-1; i++ {
		r1 := r[i]
		r2 := r[i+1]
		// fmt.Println("r1", r1, "r2", r2)
		if r1.End+1 >= r2.Start {
			maxEnd := r2.End
			if r1.End > maxEnd {
				maxEnd = r1.End
			}
			// Reduces one hole at a time. Should recurse to do more holes at a time.
			newRanges = append(append(newRanges, &blockRange{Start: r1.Start, End: maxEnd}), r[i+2:]...)
			break

		} else {
			newRanges = append(newRanges, r1)
			if i+2 == len(r) {
				newRanges = append(newRanges, r2)
			}
		}
	}
	// fmt.Println("reduce output:", newRanges)
	return newRanges
}
