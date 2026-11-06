package clickhouse

import (
	"fmt"
	"math/big"
	"strings"

	"github.com/ClickHouse/ch-go/proto"
)

// StringToDecimal128 converts a decimal string to proto.Decimal128.
// The scale parameter specifies the number of decimal places.
// For example, "123.45" with scale 2 becomes 12345 internally.
func StringToDecimal128(s string, scale int32) (proto.Decimal128, error) {
	// Remove leading/trailing whitespace
	s = strings.TrimSpace(s)
	if s == "" {
		return proto.Decimal128{}, fmt.Errorf("empty string cannot be converted to decimal")
	}

	// Handle negative sign
	negative := false
	if strings.HasPrefix(s, "-") {
		negative = true
		s = s[1:]
	} else if strings.HasPrefix(s, "+") {
		s = s[1:]
	}

	// Validate scale parameter
	if scale > 38 {
		return proto.Decimal128{}, fmt.Errorf("scale cannot exceed 38, got %d", scale)
	}
	if scale < 0 {
		return proto.Decimal128{}, fmt.Errorf("scale cannot be negative, got %d", scale)
	}

	// Split into integer and fractional parts
	parts := strings.Split(s, ".")
	if len(parts) > 2 {
		return proto.Decimal128{}, fmt.Errorf("invalid decimal format: %s", s)
	}

	integerPart := parts[0]
	fractionalPart := ""
	if len(parts) == 2 {
		fractionalPart = parts[1]
	}

	// Adjust fractional part to match the specified scale
	if len(fractionalPart) > int(scale) {
		// Truncate if too many decimal places
		fractionalPart = fractionalPart[:scale]
	} else {
		// Pad with zeros if fewer decimal places
		for len(fractionalPart) < int(scale) {
			fractionalPart += "0"
		}
	}

	// Validate that all characters are digits
	for _, r := range integerPart + fractionalPart {
		if r < '0' || r > '9' {
			return proto.Decimal128{}, fmt.Errorf("invalid character in decimal: %c", r)
		}
	}

	// Use the fractional part as-is (no padding or truncation needed)

	// Combine integer and fractional parts
	combinedStr := integerPart + fractionalPart
	if combinedStr == "" {
		combinedStr = "0"
	}

	// Convert to big.Int for handling large numbers
	bigInt := new(big.Int)
	if _, ok := bigInt.SetString(combinedStr, 10); !ok {
		return proto.Decimal128{}, fmt.Errorf("failed to parse decimal: %s", combinedStr)
	}

	// Apply negative sign if needed
	if negative {
		bigInt.Neg(bigInt)
	}

	// Check if the number fits in 128 bits (signed)
	maxDecimal128 := new(big.Int)
	maxDecimal128.Exp(big.NewInt(10), big.NewInt(38), nil) // 10^38
	minDecimal128 := new(big.Int).Neg(maxDecimal128)

	if bigInt.Cmp(maxDecimal128) >= 0 || bigInt.Cmp(minDecimal128) < 0 {
		return proto.Decimal128{}, fmt.Errorf("decimal value out of range for Decimal128: %s", s)
	}

	// Convert to Int128
	// For negative numbers, we need to handle two's complement representation
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

	return proto.Decimal128(proto.Int128{Low: low, High: high}), nil
}

// StringToDecimal256 converts a decimal string to proto.Decimal256.
// The scale parameter specifies the number of decimal places.
// For example, "123.45" with scale 2 becomes 12345 internally.
func StringToDecimal256(s string, scale int32) (proto.Decimal256, error) {
	// Remove leading/trailing whitespace
	s = strings.TrimSpace(s)
	if s == "" {
		return proto.Decimal256{}, fmt.Errorf("empty string cannot be converted to decimal")
	}

	// Handle negative sign
	negative := false
	if strings.HasPrefix(s, "-") {
		negative = true
		s = s[1:]
	} else if strings.HasPrefix(s, "+") {
		s = s[1:]
	}

	// Validate scale parameter
	if scale > 76 {
		return proto.Decimal256{}, fmt.Errorf("scale cannot exceed 76, got %d", scale)
	}
	if scale < 0 {
		return proto.Decimal256{}, fmt.Errorf("scale cannot be negative, got %d", scale)
	}

	// Split into integer and fractional parts
	parts := strings.Split(s, ".")
	if len(parts) > 2 {
		return proto.Decimal256{}, fmt.Errorf("invalid decimal format: %s", s)
	}

	integerPart := parts[0]
	fractionalPart := ""
	if len(parts) == 2 {
		fractionalPart = parts[1]
	}

	// Adjust fractional part to match the specified scale
	if len(fractionalPart) > int(scale) {
		// Truncate if too many decimal places
		fractionalPart = fractionalPart[:scale]
	} else {
		// Pad with zeros if fewer decimal places
		for len(fractionalPart) < int(scale) {
			fractionalPart += "0"
		}
	}

	// Validate that all characters are digits
	for _, r := range integerPart + fractionalPart {
		if r < '0' || r > '9' {
			return proto.Decimal256{}, fmt.Errorf("invalid character in decimal: %c", r)
		}
	}

	// Use the fractional part as-is (no padding or truncation needed)

	// Combine integer and fractional parts
	combinedStr := integerPart + fractionalPart
	if combinedStr == "" {
		combinedStr = "0"
	}

	// Convert to big.Int for handling large numbers
	bigInt := new(big.Int)
	if _, ok := bigInt.SetString(combinedStr, 10); !ok {
		return proto.Decimal256{}, fmt.Errorf("failed to parse decimal: %s", combinedStr)
	}

	// Apply negative sign if needed
	if negative {
		bigInt.Neg(bigInt)
	}

	// Check if the number fits in 256 bits (signed)
	// The maximum value for a signed 256-bit integer is 2^255 - 1
	// But for practical Decimal256 usage, we should allow very large numbers
	// Let's use a more reasonable limit based on actual 256-bit capacity
	maxDecimal256 := new(big.Int)
	maxDecimal256.Exp(big.NewInt(2), big.NewInt(255), nil) // 2^255
	maxDecimal256.Sub(maxDecimal256, big.NewInt(1))        // 2^255 - 1
	minDecimal256 := new(big.Int)
	minDecimal256.Exp(big.NewInt(2), big.NewInt(255), nil) // 2^255
	minDecimal256.Neg(minDecimal256)                       // -2^255

	if bigInt.Cmp(maxDecimal256) > 0 || bigInt.Cmp(minDecimal256) < 0 {
		return proto.Decimal256{}, fmt.Errorf("decimal value out of range for Decimal256: %s", s)
	}

	// Convert to Int256
	// For negative numbers, we need to handle two's complement representation
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
