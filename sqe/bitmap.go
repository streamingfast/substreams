package sqe

import (
	"fmt"
	"math"

	"github.com/RoaringBitmap/roaring/roaring64"
)

func RoaringBitmapsApply(expr Expression, bitmaps map[string]*roaring64.Bitmap) *roaring64.Bitmap {
	out := roaringQuerier{bitmaps: bitmaps}.apply(expr)
	if out == nil {
		return roaring64.New()
	}
	return out
}

type roaringRange struct {
	startInclusive uint64
	endExlusive    uint64
}

type roaringQuerier struct {
	bitmaps map[string]*roaring64.Bitmap

	// fullRange is computed once and is the full range of all the "block"
	// represented within the bitmaps. This is used to optimize the "not"
	// operation by flipping the full range.
	//
	// It's lazy since most expression will not use it so there is no
	// need to compute unless strictly necessary.
	fullRange *roaringRange
}

func (q roaringQuerier) apply(expr Expression) *roaring64.Bitmap {
	switch v := expr.(type) {
	case *KeyTerm:
		if out, ok := q.bitmaps[v.Value.Value]; ok {
			return out
		}
		return roaring64.New()

	case *AndExpression, *OrExpression:
		children := v.(HasChildrenExpression).GetChildren()
		if len(children) == 0 {
			panic(fmt.Errorf("%T expression with no children. this make no sense something is wrong in the parser", v))
		}

		firstChild := children[0]
		if len(children) == 1 {
			return q.apply(firstChild)
		}

		result := q.apply(firstChild).Clone()

		var op func(x2 *roaring64.Bitmap)
		switch v.(type) {
		case *AndExpression:
			op = result.And
		case *OrExpression:
			op = result.Or
		default:
			panic(fmt.Errorf("has children expression of type %T is not handled correctly", v))
		}

		for _, child := range children[1:] {
			op(q.apply(child))
		}

		return result

	case *ParenthesisExpression:
		return q.apply(v.Child)

	case *NotExpression:
		roaringRange := q.getRoaringRange()

		result := q.apply(v.Child).Clone()
		result.Flip(roaringRange.startInclusive, roaringRange.endExlusive)

		return result

	default:
		panic(fmt.Errorf("element of type %T is not handled correctly", v))
	}
}

func (q roaringQuerier) getRoaringRange() *roaringRange {
	if q.fullRange == nil {
		var start uint64 = math.MaxUint64
		var end uint64 = 0
		for _, bitmap := range q.bitmaps {
			if bitmap.IsEmpty() {
				continue
			}

			first := bitmap.Minimum()
			last := bitmap.Maximum()

			if first < start {
				start = first
			}

			if last > end {
				end = last
			}
		}

		q.fullRange = &roaringRange{
			startInclusive: start,
			endExlusive:    end + 1,
		}
	}

	return q.fullRange
}
