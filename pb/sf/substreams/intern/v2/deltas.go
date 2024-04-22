package pbssinternal

import "slices"

func (o *Operations) Sort() {
	if o == nil || o.Operations == nil {
		return
	}
	slices.SortStableFunc(o.Operations, func(a, b *Operation) int {
		if a.Ord < b.Ord {
			return -1
		}
		if a.Ord > b.Ord {
			return 1
		}
		return 0
	})
}

func (o *Operations) Add(op *Operation) {
	o.Operations = append(o.Operations, op)
}
