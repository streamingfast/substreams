package db

import (
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func Test_convertToType(t *testing.T) {

	tests := []struct {
		name      string
		value     string
		expect    any
		expectErr error
		valueType reflect.Type
	}{
		{
			name:      "Date",
			value:     "2021-01-01",
			expect:    "2021-01-01",
			expectErr: nil,
			valueType: reflect.TypeOf(time.Time{}),
		}, {
			name:      "Invalid Date",
			value:     "2021-99-01",
			expect:    nil,
			expectErr: errors.New(`could not convert 2021-99-01 to date: parsing time "2021-99-01": month out of range`),
			valueType: reflect.TypeOf(time.Time{}),
		},
		{
			name:      "ISO 8601 datetime",
			value:     "2021-01-01T00:00:00Z",
			expect:    int64(1609459200),
			expectErr: nil,
			valueType: reflect.TypeOf(time.Time{}),
		},
		{
			name:      "common datetime",
			value:     "2021-01-01 00:00:00",
			expect:    int64(1609459200),
			expectErr: nil,
			valueType: reflect.TypeOf(time.Time{}),
		},
		{
			name:      "String Slice Double Quoted",
			value:     `["field1", "field2"]`,
			expect:    []string{"field1", "field2"},
			expectErr: nil,
			valueType: reflect.TypeOf([]string{}),
		}, {
			name:      "Int Slice",
			value:     `[1, 2]`,
			expect:    []int{1, 2},
			expectErr: nil,
			valueType: reflect.TypeOf([]int{}),
		}, {
			name:      "Float Slice",
			value:     `[1.0, 2.0]`,
			expect:    []float64{1, 2},
			expectErr: nil,
			valueType: reflect.TypeOf([]float64{}),
		}, {
			name:      "Invalid Type Slice Struct",
			value:     `[""]`,
			expect:    nil,
			expectErr: errors.New(`"Time" is not supported as Clickhouse Array type`),
			valueType: reflect.TypeOf([]time.Time{}),
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			res, err := convertToType(test.value, test.valueType)
			if test.expectErr != nil {
				assert.EqualError(t, err, test.expectErr.Error())
			} else {
				assert.NoError(t, err)
				assert.Equal(t, test.expect, res)
			}
		})
	}
}
