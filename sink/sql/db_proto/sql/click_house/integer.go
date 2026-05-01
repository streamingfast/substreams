package clickhouse

import (
	"fmt"
	"math/big"
	"strings"

	"github.com/ClickHouse/ch-go/proto"
)

// StringToInt128 converts a string to proto.Int128.
func StringToInt128(s string) (proto.Int128, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return proto.Int128{}, fmt.Errorf("empty string cannot be converted to int128")
	}

	bigInt := new(big.Int)
	if _, ok := bigInt.SetString(s, 10); !ok {
		return proto.Int128{}, fmt.Errorf("invalid integer format: %s", s)
	}

	max128 := new(big.Int)
	max128.Exp(big.NewInt(2), big.NewInt(127), nil)
	max128.Sub(max128, big.NewInt(1))

	min128 := new(big.Int)
	min128.Exp(big.NewInt(2), big.NewInt(127), nil)
	min128.Neg(min128)

	if bigInt.Cmp(max128) > 0 || bigInt.Cmp(min128) < 0 {
		return proto.Int128{}, fmt.Errorf("integer value out of range for Int128: %s", s)
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

	return proto.Int128{Low: low, High: high}, nil
}

// StringToUInt128 converts a string to proto.UInt128.
func StringToUInt128(s string) (proto.UInt128, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return proto.UInt128{}, fmt.Errorf("empty string cannot be converted to uint128")
	}

	if strings.HasPrefix(s, "-") {
		return proto.UInt128{}, fmt.Errorf("negative values not allowed for UInt128: %s", s)
	}

	if strings.HasPrefix(s, "+") {
		s = s[1:]
	}

	bigInt := new(big.Int)
	if _, ok := bigInt.SetString(s, 10); !ok {
		return proto.UInt128{}, fmt.Errorf("invalid integer format: %s", s)
	}

	maxUint128 := new(big.Int)
	maxUint128.Exp(big.NewInt(2), big.NewInt(128), nil)
	maxUint128.Sub(maxUint128, big.NewInt(1))

	if bigInt.Sign() < 0 || bigInt.Cmp(maxUint128) > 0 {
		return proto.UInt128{}, fmt.Errorf("integer value out of range for UInt128: %s", s)
	}

	var low, high uint64

	low = bigInt.Uint64()
	if bigInt.BitLen() > 64 {
		bigInt.Rsh(bigInt, 64)
		high = bigInt.Uint64()
	}

	return proto.UInt128{Low: low, High: high}, nil
}

// StringToInt256 converts a string to proto.Int256.
func StringToInt256(s string) (proto.Int256, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return proto.Int256{}, fmt.Errorf("empty string cannot be converted to int256")
	}

	bigInt := new(big.Int)
	if _, ok := bigInt.SetString(s, 10); !ok {
		return proto.Int256{}, fmt.Errorf("invalid integer format: %s", s)
	}

	max256 := new(big.Int)
	max256.Exp(big.NewInt(2), big.NewInt(255), nil)
	max256.Sub(max256, big.NewInt(1))

	min256 := new(big.Int)
	min256.Exp(big.NewInt(2), big.NewInt(255), nil)
	min256.Neg(min256)

	if bigInt.Cmp(max256) > 0 || bigInt.Cmp(min256) < 0 {
		return proto.Int256{}, fmt.Errorf("integer value out of range for Int256: %s", s)
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

	return proto.Int256{
		Low: proto.UInt128{
			Low:  lowLow,
			High: lowHigh,
		},
		High: proto.UInt128{
			Low:  highLow,
			High: highHigh,
		},
	}, nil
}

// StringToUInt256 converts a string to proto.UInt256.
func StringToUInt256(s string) (proto.UInt256, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return proto.UInt256{}, fmt.Errorf("empty string cannot be converted to uint256")
	}

	if strings.HasPrefix(s, "-") {
		return proto.UInt256{}, fmt.Errorf("negative values not allowed for UInt256: %s", s)
	}

	if strings.HasPrefix(s, "+") {
		s = s[1:]
	}

	bigInt := new(big.Int)
	if _, ok := bigInt.SetString(s, 10); !ok {
		return proto.UInt256{}, fmt.Errorf("invalid integer format: %s", s)
	}

	maxUint256 := new(big.Int)
	maxUint256.Exp(big.NewInt(2), big.NewInt(256), nil)
	maxUint256.Sub(maxUint256, big.NewInt(1))

	if bigInt.Sign() < 0 || bigInt.Cmp(maxUint256) > 0 {
		return proto.UInt256{}, fmt.Errorf("integer value out of range for UInt256: %s", s)
	}

	var lowLow, lowHigh, highLow, highHigh uint64

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

	return proto.UInt256{
		Low: proto.UInt128{
			Low:  lowLow,
			High: lowHigh,
		},
		High: proto.UInt128{
			Low:  highLow,
			High: highHigh,
		},
	}, nil
}
