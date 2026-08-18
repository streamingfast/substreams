package clickhouse

import (
	"fmt"
	"math/big"
	"strings"

	"github.com/ClickHouse/ch-go/proto"
)

// StringToInt128 converts a string to proto.Int128.
// Supports decimal string representation of signed 128-bit integers.
func StringToInt128(s string) (proto.Int128, error) {
	// Remove leading/trailing whitespace
	s = strings.TrimSpace(s)
	if s == "" {
		return proto.Int128{}, fmt.Errorf("empty string cannot be converted to int128")
	}

	// Convert to big.Int for handling large numbers
	bigInt := new(big.Int)
	if _, ok := bigInt.SetString(s, 10); !ok {
		return proto.Int128{}, fmt.Errorf("invalid integer format: %s", s)
	}

	// Check if the number fits in signed 128 bits
	// Range: -2^127 to 2^127-1
	max128 := new(big.Int)
	max128.Exp(big.NewInt(2), big.NewInt(127), nil)
	max128.Sub(max128, big.NewInt(1)) // 2^127 - 1

	min128 := new(big.Int)
	min128.Exp(big.NewInt(2), big.NewInt(127), nil)
	min128.Neg(min128) // -2^127

	if bigInt.Cmp(max128) > 0 || bigInt.Cmp(min128) < 0 {
		return proto.Int128{}, fmt.Errorf("integer value out of range for Int128: %s", s)
	}

	// Convert to Int128
	var low, high uint64

	if bigInt.Sign() >= 0 {
		// Positive number
		low = bigInt.Uint64()
		if bigInt.BitLen() > 64 {
			// Number requires more than 64 bits
			bigInt.Rsh(bigInt, 64) // Right shift by 64 bits
			high = bigInt.Uint64()
		}
	} else {
		// Negative number - use two's complement
		// First get the absolute value
		absBigInt := new(big.Int).Abs(bigInt)

		// Convert to two's complement
		// For 128-bit two's complement: flip all bits and add 1
		maxUint128 := new(big.Int)
		maxUint128.SetBit(maxUint128, 128, 1) // 2^128

		twosComplement := new(big.Int).Sub(maxUint128, absBigInt)

		low = twosComplement.Uint64()
		if twosComplement.BitLen() > 64 {
			twosComplement.Rsh(twosComplement, 64)
			high = twosComplement.Uint64()
		} else {
			high = ^uint64(0) // All bits set for negative number
		}
	}

	return proto.Int128{Low: low, High: high}, nil
}

// StringToUInt128 converts a string to proto.UInt128.
// Supports decimal string representation of unsigned 128-bit integers.
func StringToUInt128(s string) (proto.UInt128, error) {
	// Remove leading/trailing whitespace
	s = strings.TrimSpace(s)
	if s == "" {
		return proto.UInt128{}, fmt.Errorf("empty string cannot be converted to uint128")
	}

	// Handle negative sign - not allowed for unsigned integers
	if strings.HasPrefix(s, "-") {
		return proto.UInt128{}, fmt.Errorf("negative values not allowed for UInt128: %s", s)
	}

	// Remove optional positive sign
	if strings.HasPrefix(s, "+") {
		s = s[1:]
	}

	// Convert to big.Int for handling large numbers
	bigInt := new(big.Int)
	if _, ok := bigInt.SetString(s, 10); !ok {
		return proto.UInt128{}, fmt.Errorf("invalid integer format: %s", s)
	}

	// Check if the number fits in unsigned 128 bits
	// Range: 0 to 2^128-1
	maxUint128 := new(big.Int)
	maxUint128.Exp(big.NewInt(2), big.NewInt(128), nil)
	maxUint128.Sub(maxUint128, big.NewInt(1)) // 2^128 - 1

	if bigInt.Sign() < 0 || bigInt.Cmp(maxUint128) > 0 {
		return proto.UInt128{}, fmt.Errorf("integer value out of range for UInt128: %s", s)
	}

	// Convert to UInt128
	var low, high uint64

	// Extract low 64 bits
	low = bigInt.Uint64()
	if bigInt.BitLen() > 64 {
		// Extract high 64 bits
		bigInt.Rsh(bigInt, 64)
		high = bigInt.Uint64()
	}

	return proto.UInt128{Low: low, High: high}, nil
}

// StringToInt256 converts a string to proto.Int256.
// Supports decimal string representation of signed 256-bit integers.
func StringToInt256(s string) (proto.Int256, error) {
	// Remove leading/trailing whitespace
	s = strings.TrimSpace(s)
	if s == "" {
		return proto.Int256{}, fmt.Errorf("empty string cannot be converted to int256")
	}

	// Convert to big.Int for handling large numbers
	bigInt := new(big.Int)
	if _, ok := bigInt.SetString(s, 10); !ok {
		return proto.Int256{}, fmt.Errorf("invalid integer format: %s", s)
	}

	// Check if the number fits in signed 256 bits
	// Range: -2^255 to 2^255-1
	max256 := new(big.Int)
	max256.Exp(big.NewInt(2), big.NewInt(255), nil)
	max256.Sub(max256, big.NewInt(1)) // 2^255 - 1

	min256 := new(big.Int)
	min256.Exp(big.NewInt(2), big.NewInt(255), nil)
	min256.Neg(min256) // -2^255

	if bigInt.Cmp(max256) > 0 || bigInt.Cmp(min256) < 0 {
		return proto.Int256{}, fmt.Errorf("integer value out of range for Int256: %s", s)
	}

	// Convert to Int256
	var lowLow, lowHigh, highLow, highHigh uint64

	if bigInt.Sign() >= 0 {
		// Positive number - extract 64-bit chunks
		tempBig := new(big.Int).Set(bigInt)

		// Extract low.low (bits 0-63)
		lowLow = tempBig.Uint64()
		tempBig.Rsh(tempBig, 64)

		// Extract low.high (bits 64-127)
		if tempBig.BitLen() > 0 {
			lowHigh = tempBig.Uint64()
			tempBig.Rsh(tempBig, 64)
		}

		// Extract high.low (bits 128-191)
		if tempBig.BitLen() > 0 {
			highLow = tempBig.Uint64()
			tempBig.Rsh(tempBig, 64)
		}

		// Extract high.high (bits 192-255)
		if tempBig.BitLen() > 0 {
			highHigh = tempBig.Uint64()
		}
	} else {
		// Negative number - use two's complement
		// First get the absolute value
		absBigInt := new(big.Int).Abs(bigInt)

		// Convert to two's complement
		// For 256-bit two's complement: flip all bits and add 1
		maxUint256 := new(big.Int)
		maxUint256.SetBit(maxUint256, 256, 1) // 2^256

		twosComplement := new(big.Int).Sub(maxUint256, absBigInt)

		// Extract 64-bit chunks
		tempBig := new(big.Int).Set(twosComplement)

		// Extract low.low (bits 0-63)
		lowLow = tempBig.Uint64()
		tempBig.Rsh(tempBig, 64)

		// Extract low.high (bits 64-127)
		if tempBig.BitLen() > 0 {
			lowHigh = tempBig.Uint64()
			tempBig.Rsh(tempBig, 64)
		} else {
			lowHigh = ^uint64(0) // All bits set for negative number
		}

		// Extract high.low (bits 128-191)
		if tempBig.BitLen() > 0 {
			highLow = tempBig.Uint64()
			tempBig.Rsh(tempBig, 64)
		} else {
			highLow = ^uint64(0) // All bits set for negative number
		}

		// Extract high.high (bits 192-255)
		if tempBig.BitLen() > 0 {
			highHigh = tempBig.Uint64()
		} else {
			highHigh = ^uint64(0) // All bits set for negative number
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
// Supports decimal string representation of unsigned 256-bit integers.
func StringToUInt256(s string) (proto.UInt256, error) {
	// Remove leading/trailing whitespace
	s = strings.TrimSpace(s)
	if s == "" {
		return proto.UInt256{}, fmt.Errorf("empty string cannot be converted to uint256")
	}

	// Handle negative sign - not allowed for unsigned integers
	if strings.HasPrefix(s, "-") {
		return proto.UInt256{}, fmt.Errorf("negative values not allowed for UInt256: %s", s)
	}

	// Remove optional positive sign
	if strings.HasPrefix(s, "+") {
		s = s[1:]
	}

	// Convert to big.Int for handling large numbers
	bigInt := new(big.Int)
	if _, ok := bigInt.SetString(s, 10); !ok {
		return proto.UInt256{}, fmt.Errorf("invalid integer format: %s", s)
	}

	// Check if the number fits in unsigned 256 bits
	// Range: 0 to 2^256-1
	maxUint256 := new(big.Int)
	maxUint256.Exp(big.NewInt(2), big.NewInt(256), nil)
	maxUint256.Sub(maxUint256, big.NewInt(1)) // 2^256 - 1

	if bigInt.Sign() < 0 || bigInt.Cmp(maxUint256) > 0 {
		return proto.UInt256{}, fmt.Errorf("integer value out of range for UInt256: %s", s)
	}

	// Convert to UInt256 - extract 64-bit chunks
	var lowLow, lowHigh, highLow, highHigh uint64

	tempBig := new(big.Int).Set(bigInt)

	// Extract low.low (bits 0-63)
	lowLow = tempBig.Uint64()
	tempBig.Rsh(tempBig, 64)

	// Extract low.high (bits 64-127)
	if tempBig.BitLen() > 0 {
		lowHigh = tempBig.Uint64()
		tempBig.Rsh(tempBig, 64)
	}

	// Extract high.low (bits 128-191)
	if tempBig.BitLen() > 0 {
		highLow = tempBig.Uint64()
		tempBig.Rsh(tempBig, 64)
	}

	// Extract high.high (bits 192-255)
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
