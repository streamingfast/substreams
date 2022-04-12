package entity

import "math/big"

var ONE_BI = new(big.Int).SetUint64(1)

func S(str string) *string {
	return &str
}

func B(v bool) *bool {
	return &v
}

func Inc(v Int) Int {
	if v.int == nil {
		return NewInt(ONE_BI)
	}

	return NewInt(new(big.Int).Add(v.int, ONE_BI))
}
