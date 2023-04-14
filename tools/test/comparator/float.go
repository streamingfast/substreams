package comparator

import (
	"fmt"
	"math/big"
	"net/url"
)

var _ Comparable = (*Float)(nil)

type Float struct {
	expect *big.Float
	error  *big.Float
}

func newFloat(expect string, args url.Values) (*Float, error) {
	expected, ok := new(big.Float).SetString(expect)
	if !ok {
		return nil, fmt.Errorf("unable to parse expected float value")
	}
	f := &Float{expect: expected}
	if error := args.Get("error"); error != "" {
		f.error, ok = new(big.Float).SetString(error)
		if !ok {
			return nil, fmt.Errorf("unable to parse float precision")
		}
	}

	return f, nil
}

func (i *Float) Cmp(actual string) (bool, string, error) {
	a, ok := new(big.Float).SetString(actual)
	if !ok {
		return false, "", fmt.Errorf("[float] failed to parse %q as big float", actual)
	}

	if i.expect.Cmp(a) == 0 {
		return true, "", nil
	}

	if i.error == nil {
		return false, fmt.Sprintf("[float] expected %q to equal %q", a.String(), i.expect.String()), nil
	}

	dt := new(big.Float).Add(i.expect, new(big.Float).Mul(a, new(big.Float).SetInt64(-1)))
	if (dt.Cmp(i.error) > 0) || dt.Cmp(new(big.Float).Mul(i.error, new(big.Float).SetInt64(-1))) < 0 {
		return false, fmt.Sprintf("[float] expected  %q to equal  %q within error: %s", a.String(), i.expect.String(), i.error.String()), nil
	}
	return true, "", nil
}
