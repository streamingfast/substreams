package comparator

import (
	"fmt"
	"math/big"
	"net/url"
)

var _ Comparable = (*Int)(nil)

type Int struct {
	expect *big.Int
}

func newInt(expect string, args url.Values) (*Int, error) {
	int, ok := new(big.Int).SetString(expect, 10)
	if !ok {
		return nil, fmt.Errorf("failed to setup Integer compare")
	}
	return &Int{expect: int}, nil
}

func (i *Int) Cmp(actual string) (bool, string, error) {
	a, ok := new(big.Int).SetString(actual, 10)
	if !ok {
		return false, "", fmt.Errorf("[int] failed to parse %q as big int", actual)
	}

	if i.expect.Cmp(a) != 0 {
		return false, fmt.Sprintf("[int] expected %q to equal %q", a.String(), i.expect.String()), nil
	}
	return true, "", nil
}
