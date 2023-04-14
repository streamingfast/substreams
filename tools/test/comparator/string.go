package comparator

import (
	"fmt"
	"net/url"
)

var _ Comparable = (*String)(nil)

type String struct {
	expect string
}

func newString(expect string, args url.Values) *String {
	return &String{expect: expect}
}

func (s *String) Cmp(actual string) (bool, string, error) {
	if actual != s.expect {
		return false, fmt.Sprintf("[string] expected %q to equal %q", actual, s.expect), nil
	}
	return true, "", nil
}
