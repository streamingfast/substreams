package entity

import (
	"math/big"
)

func ConvertTokenToDecimal(amount *big.Int, decimals int64) *big.Float {
	a := new(big.Float).SetInt(amount).SetPrec(100)
	if decimals == 0 {
		return a
	}

	return a.Quo(a, ExponentToBigFloat(decimals).SetPrec(100)).SetPrec(100)
}

func ExponentToBigFloat(decimals int64) *big.Float {
	bd := new(big.Float).SetInt64(1)
	ten := new(big.Float).SetInt64(10)
	for i := int64(0); i < decimals; i++ {
		bd = bd.Mul(bd, ten)
	}
	return bd
}
