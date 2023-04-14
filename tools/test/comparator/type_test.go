package comparator

import (
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"reflect"
	"testing"
)

func TestNewComparable(t *testing.T) {
	tests := []struct {
		name        string
		expect      string
		op          string
		args        string
		expectType  reflect.Type
		expectError bool
	}{
		{"default to string op", "helloworld", "", "", reflect.TypeOf(&String{}), false},
		{"use the op", "helloworld", "string", "", reflect.TypeOf(&String{}), false},
		{"fails if expect not float", "adsa", "float", "", nil, true},
		{"fails if expect not int", "adsa", "int", "", nil, true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			out, err := NewComparable(test.expect, test.op, test.args)
			if test.expectError {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, test.expectType, reflect.TypeOf(out))
			}
		})
	}
}
