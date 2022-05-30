package tui

import pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"

func newModel() model {
	return model{
		Modules: updatedRanges{},
	}
}

type model struct {
	Modules      updatedRanges
	DebugSetting bool
	Updates      int

	Connected bool

	Failures int
	Reason   string
	Clock    *pbsubstreams.Clock
}

type ranges []blockRange

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
		if lo >= blockRange.Start && hi <= blockRange.End {
			return true
		}
	}
	return false
}

type blockRange struct {
	Start uint64
	End   uint64
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
