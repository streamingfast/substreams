package entity

import (
	"math/big"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestConvertTokenToDecimal(t *testing.T) {
	var cases = []struct {
		name     string
		expected *big.Float
		decimals int64
		amount   *big.Int
	}{
		{
			name:     "1",
			amount:   new(big.Int).SetInt64(1),
			decimals: 0,
			expected: new(big.Float).SetInt64(1),
		},
		{
			name:     "100",
			amount:   new(big.Int).SetInt64(100),
			decimals: 2,
			expected: new(big.Float).SetInt64(1.00),
		},
		{
			name:     "1000",
			amount:   new(big.Int).SetInt64(1000),
			decimals: 2,
			expected: new(big.Float).SetInt64(10.0),
		},
		{
			name:     "0.000000000000000001",
			amount:   new(big.Int).SetInt64(1),
			decimals: 18,
			expected: new(big.Float).SetFloat64(0.000000000000000001),
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			bd := ConvertTokenToDecimal(c.amount, c.decimals)
			require.Equal(t, c.expected.String(), bd.String())
		})
	}
}
