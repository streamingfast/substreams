package clickhouse

import (
	"fmt"
	"math/big"
	"strings"

	"github.com/ClickHouse/ch-go/proto"
)

// StringToDecimal128 converts a decimal string to proto.Decimal128.
// The scale parameter specifies the number of decimal places.
func StringToDecimal128(s string, scale int32) (proto.Decimal128, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return proto.Decimal128{}, fmt.Errorf("empty string cannot be converted to decimal")
	}

	negative := false
	if strings.HasPrefix(s, "-") {
		negative = true
		s = s[1:]
	} else if strings.HasPrefix(s, "+") {
		s = s[1:]
	}

	if scale > 38 {
		return proto.Decimal128{}, fmt.Errorf("scale cannot exceed 38, got %d", scale)
	}
	if scale < 0 {
		return proto.Decimal128{}, fmt.Errorf("scale cannot be negative, got %d", scale)
	}

	parts := strings.Split(s, ".")
	if len(parts) > 2 {
		return proto.Decimal128{}, fmt.Errorf("invalid decimal format: %s", s)
	}

	integerPart := parts[0]
	fractionalPart := ""
	if len(parts) == 2 {
		fractionalPart = parts[1]
	}

	if len(fractionalPart) > int(scale) {
		fractionalPart = fractionalPart[:scale]
	} else {
		for len(fractionalPart) < int(scale) {
			fractionalPart += "0"
		}
	}

	for _, r := range integerPart + fractionalPart {
		if r < '0' || r > '9' {
			return proto.Decimal128{}, fmt.Errorf("invalid character in decimal: %c", r)
		}
	}

	combinedStr := integerPart + fractionalPart
	if combinedStr == "" {
		combinedStr = "0"
	}

	bigInt := new(big.Int)
	if _, ok := bigInt.SetString(combinedStr, 10); !ok {
		return proto.Decimal128{}, fmt.Errorf("failed to parse decimal: %s", combinedStr)
	}

	if negative {
		bigInt.Neg(bigInt)
	}

	maxDecimal128 := new(big.Int)
	maxDecimal128.Exp(big.NewInt(10), big.NewInt(38), nil)
	minDecimal128 := new(big.Int).Neg(maxDecimal128)

	if bigInt.Cmp(maxDecimal128) >= 0 || bigInt.Cmp(minDecimal128) < 0 {
		return proto.Decimal128{}, fmt.Errorf("decimal value out of range for Decimal128: %s", s)
	}

	var low, high uint64

	if bigInt.Sign() >= 0 {
		low = bigInt.Uint64()
		if bigInt.BitLen() > 64 {
			bigInt.Rsh(bigInt, 64)
			high = bigInt.Uint64()
		}
	} else {
		absBigInt := new(big.Int).Abs(bigInt)
		maxUint128 := new(big.Int)
		maxUint128.SetBit(maxUint128, 128, 1)
		twosComplement := new(big.Int).Sub(maxUint128, absBigInt)
		low = twosComplement.Uint64()
		if twosComplement.BitLen() > 64 {
			twosComplement.Rsh(twosComplement, 64)
			high = twosComplement.Uint64()
		} else {
			high = ^uint64(0)
		}
	}

	return proto.Decimal128(proto.Int128{Low: low, High: high}), nil
}

// StringToDecimal256 converts a decimal string to proto.Decimal256.
func StringToDecimal256(s string, scale int32) (proto.Decimal256, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return proto.Decimal256{}, fmt.Errorf("empty string cannot be converted to decimal")
	}

	negative := false
	if strings.HasPrefix(s, "-") {
		negative = true
		s = s[1:]
	} else if strings.HasPrefix(s, "+") {
		s = s[1:]
	}

	if scale > 76 {
		return proto.Decimal256{}, fmt.Errorf("scale cannot exceed 76, got %d", scale)
	}
	if scale < 0 {
		return proto.Decimal256{}, fmt.Errorf("scale cannot be negative, got %d", scale)
	}

	parts := strings.Split(s, ".")
	if len(parts) > 2 {
		return proto.Decimal256{}, fmt.Errorf("invalid decimal format: %s", s)
	}

	integerPart := parts[0]
	fractionalPart := ""
	if len(parts) == 2 {
		fractionalPart = parts[1]
	}

	if len(fractionalPart) > int(scale) {
		fractionalPart = fractionalPart[:scale]
	} else {
		for len(fractionalPart) < int(scale) {
			fractionalPart += "0"
		}
	}

	for _, r := range integerPart + fractionalPart {
		if r < '0' || r > '9' {
			return proto.Decimal256{}, fmt.Errorf("invalid character in decimal: %c", r)
		}
	}

	combinedStr := integerPart + fractionalPart
	if combinedStr == "" {
		combinedStr = "0"
	}

	bigInt := new(big.Int)
	if _, ok := bigInt.SetString(combinedStr, 10); !ok {
		return proto.Decimal256{}, fmt.Errorf("failed to parse decimal: %s", combinedStr)
	}

	if negative {
		bigInt.Neg(bigInt)
	}

	maxDecimal256 := new(big.Int)
	maxDecimal256.Exp(big.NewInt(2), big.NewInt(255), nil)
	maxDecimal256.Sub(maxDecimal256, big.NewInt(1))
	minDecimal256 := new(big.Int)
	minDecimal256.Exp(big.NewInt(2), big.NewInt(255), nil)
	minDecimal256.Neg(minDecimal256)

	if bigInt.Cmp(maxDecimal256) > 0 || bigInt.Cmp(minDecimal256) < 0 {
		return proto.Decimal256{}, fmt.Errorf("decimal value out of range for Decimal256: %s", s)
	}

	var lowLow, lowHigh, highLow, highHigh uint64

	if bigInt.Sign() >= 0 {
		tempBig := new(big.Int).Set(bigInt)
		lowLow = tempBig.Uint64()
		tempBig.Rsh(tempBig, 64)
		if tempBig.BitLen() > 0 {
			lowHigh = tempBig.Uint64()
			tempBig.Rsh(tempBig, 64)
		}
		if tempBig.BitLen() > 0 {
			highLow = tempBig.Uint64()
			tempBig.Rsh(tempBig, 64)
		}
		if tempBig.BitLen() > 0 {
			highHigh = tempBig.Uint64()
		}
	} else {
		absBigInt := new(big.Int).Abs(bigInt)
		maxUint256 := new(big.Int)
		maxUint256.SetBit(maxUint256, 256, 1)
		twosComplement := new(big.Int).Sub(maxUint256, absBigInt)
		tempBig := new(big.Int).Set(twosComplement)
		lowLow = tempBig.Uint64()
		tempBig.Rsh(tempBig, 64)
		if tempBig.BitLen() > 0 {
			lowHigh = tempBig.Uint64()
			tempBig.Rsh(tempBig, 64)
		} else {
			lowHigh = ^uint64(0)
		}
		if tempBig.BitLen() > 0 {
			highLow = tempBig.Uint64()
			tempBig.Rsh(tempBig, 64)
		} else {
			highLow = ^uint64(0)
		}
		if tempBig.BitLen() > 0 {
			highHigh = tempBig.Uint64()
		} else {
			highHigh = ^uint64(0)
		}
	}

	return proto.Decimal256(proto.Int256{
		Low: proto.UInt128{
			Low:  lowLow,
			High: lowHigh,
		},
		High: proto.UInt128{
			Low:  highLow,
			High: highHigh,
		},
	}), nil
}
