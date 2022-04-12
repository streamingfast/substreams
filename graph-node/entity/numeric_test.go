package entity

import (
	"encoding/json"
	"fmt"
	"math/big"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEncoding(t *testing.T) {
	reserve0, _ := new(big.Int).SetString("45765335860512456044", 10)
	reserve1, _ := new(big.Int).SetString("74892810941332951", 10)
	tokenDecimal := NewIntFromLiteral(18)
	reserver0WithDecimal := NewFloat(ConvertTokenToDecimal(reserve0, tokenDecimal.Int().Int64()))
	reserver1WithDecimal := NewFloat(ConvertTokenToDecimal(reserve1, tokenDecimal.Int().Int64()))
	price := NewFloat(new(big.Float).Quo(reserver0WithDecimal.Float(), reserver1WithDecimal.Float()))

	cnt, err := json.Marshal(price)
	require.NoError(t, err)

	readPrice := &Float{}
	err = json.Unmarshal(cnt, &readPrice)
	require.NoError(t, err)
	fmt.Println(readPrice)
}
func TestExponentToBigDecimal(t *testing.T) {

	var cases = []struct {
		name     string
		expected *big.Float
		decimals int64
	}{
		{
			name:     "0",
			decimals: 0,
			expected: new(big.Float).SetInt64(1),
		},
		{
			name:     "1",
			decimals: 1,
			expected: new(big.Float).SetInt64(10),
		},
		{
			name:     "2",
			decimals: 2,
			expected: new(big.Float).SetInt64(100),
		},
		{
			name:     "18",
			decimals: 18,
			expected: new(big.Float).SetInt64(1000000000000000000),
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			bd := ExponentToBigFloat(c.decimals)
			require.Equal(t, c.expected, bd)
		})
	}
}

func TestFloatMul(t *testing.T) {
	t.Skip("precision issue")
	reserve0, _ := new(big.Int).SetString("45765335860512456044", 10)
	reserve1, _ := new(big.Int).SetString("74892810941332951", 10)
	tokenDecimal := NewIntFromLiteral(18)
	reserver0WithDecimal := NewFloat(ConvertTokenToDecimal(reserve0, tokenDecimal.Int().Int64()))
	reserver1WithDecimal := NewFloat(ConvertTokenToDecimal(reserve1, tokenDecimal.Int().Int64()))
	price := NewFloat(new(big.Float).Quo(reserver0WithDecimal.Float(), reserver1WithDecimal.Float()))
	assert.Equal(t, "611.0778228949449984270327371611629", price.StringRounded(34))
}

func TestPrecisionComparedToRust(t *testing.T) {
	//tests := []struct {
	//	input  string
	//	op     func(f Float) Float
	//	expect string
	//}{
	//	{
	//		input: "4.52323432423",
	//		op: func(f Float) Float {
	//			other, _, _ := big.ParseFloat("1.845555555555545454", 10, 64, big.ToNearestEven)
	//			return NewFloat(new(big.Float).Mul(f.Float(), other))
	//		},
	//		expect: "8.347880236162209864",
	//	},
	//	{
	//		input: "4.52323432423",
	//		op: func(f Float) Float {
	//			other, _, _ := big.ParseFloat("1.5", 10, 64, big.ToNearestEven)
	//			return NewFloat(new(big.Float).Mul(f.Float(), other))
	//		},
	//		expect: "6.784851486345",
	//	},
	//}
	//
	//for idx, test := range tests {
	//	t.Run(fmt.Sprintf("idx%d", idx+1), func(t *testing.T) {
	//		var jsonF Float
	//		require.NoError(t, json.Unmarshal([]byte(`"`+test.input+`"`), &jsonF))
	//		assert.Equal(t, test.expect, test.op(jsonF).String())
	//
	//		var pqF Float
	//		require.NoError(t, pqF.Scan([]byte(test.input)))
	//		assert.Equal(t, test.expect, test.op(pqF).String())
	//	})
	//}

}
