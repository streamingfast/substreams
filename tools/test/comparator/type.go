package comparator

import (
	"fmt"
	"net/url"
)

type Comparable interface {
	Cmp(actual string) (bool, string, error)
}

func NewComparable(expect string, op string, args string) (cmp Comparable, err error) {
	// default to string operation
	if op == "" {
		op = "string"
	}

	params, err := url.ParseQuery(args)
	if err != nil {
		return nil, fmt.Errorf("failed to parse arg as url query string: %w", err)
	}

	switch op {
	case "string":
		cmp = newString(expect, params)
	case "int":
		cmp, err = newInt(expect, params)
	case "float":
		cmp, err = newFloat(expect, params)
	}
	if err != nil {
		return nil, fmt.Errorf("unknown op %q", op)
	}
	return cmp, nil
}

type Def struct {
	Op     string `json:"op" yaml:"op"`
	Expect string `json:"expect" yaml:"expect"`

	Config map[string]string `json:"config,omitempty" yaml:"config"`
}
