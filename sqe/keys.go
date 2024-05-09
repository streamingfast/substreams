package sqe

import (
	"fmt"

	pbindex "github.com/streamingfast/substreams/pb/sf/substreams/index/v1"
)

type KeysQuerier struct {
	blockKeys map[string]struct{}
}

func NewFromIndexKeys(indexKeys *pbindex.Keys) KeysQuerier {
	blockKeys := make(map[string]struct{}, len(indexKeys.Keys))
	for _, key := range indexKeys.Keys {
		blockKeys[key] = struct{}{}
	}

	return KeysQuerier{blockKeys: blockKeys}
}
func KeysApply(expr Expression, blockKeys KeysQuerier) bool {
	return blockKeys.apply(expr)
}

func (k KeysQuerier) apply(expr Expression) bool {
	switch v := expr.(type) {
	case *KeyTerm:
		if k.blockKeys == nil {
			return false
		}

		_, ok := k.blockKeys[v.Value.Value]
		return ok

	case *AndExpression, *OrExpression:
		children := v.(HasChildrenExpression).GetChildren()
		if len(children) == 0 {
			panic(fmt.Errorf("%T expression with no children. this make no sense something is wrong in the parser", v))
		}

		firstChild := children[0]
		if len(children) == 1 {
			return k.apply(firstChild)
		}

		result := k.apply(firstChild)

		var op func(bool)
		switch v.(type) {
		case *AndExpression:
			op = func(x bool) {
				result = result && x
			}

		case *OrExpression:
			op = func(x bool) {
				result = result || x
			}
		default:
			panic(fmt.Errorf("has children expression of type %T is not handled correctly", v))
		}

		for _, child := range children[1:] {
			op(k.apply(child))
		}

		return result

	case *ParenthesisExpression:
		return k.apply(v.Child)

	case *NotExpression:
		if k.blockKeys == nil {
			return false
		}

		return !k.apply(v.Child)

	default:
		panic(fmt.Errorf("element of type %T is not handled correctly", v))
	}
}
