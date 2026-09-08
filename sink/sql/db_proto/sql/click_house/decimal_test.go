package clickhouse

import (
	"strings"
	"testing"

	"github.com/ClickHouse/ch-go/proto"
	"github.com/stretchr/testify/assert"
)

// getScaleFromInput determines the scale from a decimal input string
// This maintains backward compatibility with the original test expectations
func getScaleFromInput(input string) int32 {
	input = strings.TrimSpace(input)

	// Handle edge cases
	if input == "" || input == "+" || input == "-" || input == "." {
		return 0
	}

	// Remove sign
	if strings.HasPrefix(input, "+") || strings.HasPrefix(input, "-") {
		input = input[1:]
	}

	// Find decimal point
	parts := strings.Split(input, ".")
	if len(parts) <= 1 {
		return 0 // No decimal point
	}

	return int32(len(parts[1])) // Length of fractional part
}

func TestStringToDecimal128(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		expected  proto.Decimal128
		expectErr bool
		errMsg    string
	}{
		// Valid cases
		{
			name:      "Basic positive decimal",
			input:     "123.45",
			expected:  proto.Decimal128(proto.Int128{Low: 12345, High: 0}),
			expectErr: false,
		},
		{
			name:      "Basic negative decimal",
			input:     "-123.45",
			expected:  proto.Decimal128(proto.Int128{Low: ^uint64(12345 - 1), High: ^uint64(0)}),
			expectErr: false,
		},
		{
			name:      "Zero",
			input:     "0",
			expected:  proto.Decimal128(proto.Int128{Low: 0, High: 0}),
			expectErr: false,
		},
		{
			name:      "Zero with decimal",
			input:     "0.00",
			expected:  proto.Decimal128(proto.Int128{Low: 0, High: 0}),
			expectErr: false,
		},
		{
			name:      "Integer without decimal point",
			input:     "1000",
			expected:  proto.Decimal128(proto.Int128{Low: 1000, High: 0}),
			expectErr: false,
		},
		{
			name:      "Small decimal with higher scale",
			input:     "0.001",
			expected:  proto.Decimal128(proto.Int128{Low: 1, High: 0}),
			expectErr: false,
		},
		{
			name:      "Decimal with all digits",
			input:     "123.456789",
			expected:  proto.Decimal128(proto.Int128{Low: 123456789, High: 0}),
			expectErr: false,
		},
		{
			name:      "Decimal with trailing zero",
			input:     "123.4",
			expected:  proto.Decimal128(proto.Int128{Low: 1234, High: 0}),
			expectErr: false,
		},
		{
			name:      "Positive sign",
			input:     "+123.45",
			expected:  proto.Decimal128(proto.Int128{Low: 12345, High: 0}),
			expectErr: false,
		},
		{
			name:      "Leading/trailing whitespace",
			input:     "  123.45  ",
			expected:  proto.Decimal128(proto.Int128{Low: 12345, High: 0}),
			expectErr: false,
		},
		{
			name:      "Integer only",
			input:     "123",
			expected:  proto.Decimal128(proto.Int128{Low: 123, High: 0}),
			expectErr: false,
		},
		{
			name:      "Large positive number",
			input:     "999999999999999999999999999999999999",
			expected:  proto.Decimal128(proto.Int128{Low: 12919594847110692863, High: 54210108624275221}),
			expectErr: false,
		},

		// Error cases
		{
			name:      "Empty string",
			input:     "",
			expectErr: true,
			errMsg:    "empty string cannot be converted to decimal",
		},
		{
			name:      "Only whitespace",
			input:     "   ",
			expectErr: true,
			errMsg:    "empty string cannot be converted to decimal",
		},
		{
			name:      "Scale too high",
			input:     "1." + strings.Repeat("1", 39), // 39 decimal places
			expectErr: true,
			errMsg:    "scale cannot exceed 38",
		},
		{
			name:      "Invalid decimal format - multiple dots",
			input:     "123.45.67",
			expectErr: true,
			errMsg:    "invalid decimal format: 123.45.67",
		},
		{
			name:      "Invalid character - letter",
			input:     "123.4a",
			expectErr: true,
			errMsg:    "invalid character in decimal: a",
		},
		{
			name:      "Invalid character - special symbol",
			input:     "123.4$",
			expectErr: true,
			errMsg:    "invalid character in decimal: $",
		},
		{
			name:      "Number too large for Decimal128",
			input:     "100000000000000000000000000000000000000",
			expectErr: true,
			errMsg:    "decimal value out of range for Decimal128",
		},
		{
			name:      "Only negative sign",
			input:     "-",
			expectErr: false,
			expected:  proto.Decimal128(proto.Int128{Low: 0, High: 0}),
		},
		{
			name:      "Only positive sign",
			input:     "+",
			expectErr: false,
			expected:  proto.Decimal128(proto.Int128{Low: 0, High: 0}),
		},
		{
			name:      "Dot only",
			input:     ".",
			expectErr: false,
			expected:  proto.Decimal128(proto.Int128{Low: 0, High: 0}),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Determine scale from input string for backward compatibility
			scale := getScaleFromInput(tt.input)
			result, err := StringToDecimal128(tt.input, scale)

			if tt.expectErr {
				assert.Error(t, err)
				if tt.errMsg != "" && err != nil {
					assert.Contains(t, err.Error(), tt.errMsg)
				}
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expected, result)
			}
		})
	}
}

func TestStringToDecimal128_EdgeCases(t *testing.T) {
	// Test very small decimal
	t.Run("Very small positive decimal", func(t *testing.T) {
		result, err := StringToDecimal128("0.000000000000000001", 18) // 18 decimal places
		assert.NoError(t, err)
		expected := proto.Decimal128(proto.Int128{Low: 1, High: 0})
		assert.Equal(t, expected, result)
	})

	// Test very small negative decimal
	t.Run("Very small negative decimal", func(t *testing.T) {
		result, err := StringToDecimal128("-0.000000000000000001", 18) // 18 decimal places
		assert.NoError(t, err)
		expected := proto.Decimal128(proto.Int128{Low: ^uint64(0), High: ^uint64(0)})
		assert.Equal(t, expected, result)
	})

	// Test integer with implicit scale 0
	t.Run("Integer with implicit scale 0", func(t *testing.T) {
		result, err := StringToDecimal128("1", 0) // Scale 0 for integer
		assert.NoError(t, err)
		expected := proto.Decimal128(proto.Int128{Low: 1, High: 0})
		assert.Equal(t, expected, result)
	})
}

func TestStringToDecimal256(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		expected  proto.Decimal256
		expectErr bool
		errMsg    string
	}{
		{
			name:     "Simple positive decimal",
			input:    "123.45",
			expected: proto.Decimal256(proto.Int256{Low: proto.UInt128{Low: 12345, High: 0}, High: proto.UInt128{Low: 0, High: 0}}),
		},
		{
			name:     "Simple negative decimal",
			input:    "-123.45",
			expected: proto.Decimal256(proto.Int256{Low: proto.UInt128{Low: ^uint64(12344), High: ^uint64(0)}, High: proto.UInt128{Low: ^uint64(0), High: ^uint64(0)}}),
		},
		{
			name:     "Zero value",
			input:    "0",
			expected: proto.Decimal256(proto.Int256{Low: proto.UInt128{Low: 0, High: 0}, High: proto.UInt128{Low: 0, High: 0}}),
		},
		{
			name:     "Positive integer no decimal",
			input:    "12345",
			expected: proto.Decimal256(proto.Int256{Low: proto.UInt128{Low: 12345, High: 0}, High: proto.UInt128{Low: 0, High: 0}}),
		},
		{
			name:     "Large positive number",
			input:    "12345678901234567890",
			expected: proto.Decimal256(proto.Int256{Low: proto.UInt128{Low: 12345678901234567890, High: 0}, High: proto.UInt128{Low: 0, High: 0}}),
		},
		{
			name:     "Decimal with trailing zeros",
			input:    "123.450",
			expected: proto.Decimal256(proto.Int256{Low: proto.UInt128{Low: 123450, High: 0}, High: proto.UInt128{Low: 0, High: 0}}),
		},
		{
			name:     "Decimal with leading whitespace",
			input:    "  123.45",
			expected: proto.Decimal256(proto.Int256{Low: proto.UInt128{Low: 12345, High: 0}, High: proto.UInt128{Low: 0, High: 0}}),
		},
		{
			name:     "Decimal with positive sign",
			input:    "+123.45",
			expected: proto.Decimal256(proto.Int256{Low: proto.UInt128{Low: 12345, High: 0}, High: proto.UInt128{Low: 0, High: 0}}),
		},
		{
			name:      "Empty string",
			input:     "",
			expectErr: true,
			errMsg:    "empty string cannot be converted to decimal",
		},
		{
			name:      "Scale too high",
			input:     "1." + strings.Repeat("1", 77), // 77 decimal places
			expectErr: true,
			errMsg:    "scale cannot exceed 76",
		},
		{
			name:      "Invalid decimal format - multiple dots",
			input:     "123.45.67",
			expectErr: true,
			errMsg:    "invalid decimal format: 123.45.67",
		},
		{
			name:      "Invalid character - letter",
			input:     "123.4a",
			expectErr: true,
			errMsg:    "invalid character in decimal: a",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Determine scale from input string for backward compatibility
			scale := getScaleFromInput(tt.input)
			result, err := StringToDecimal256(tt.input, scale)

			if tt.expectErr {
				assert.Error(t, err)
				if tt.errMsg != "" && err != nil {
					assert.Contains(t, err.Error(), tt.errMsg)
				}
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expected, result)
			}
		})
	}
}

func TestStringToDecimal256_EdgeCases(t *testing.T) {
	// Test very small decimal
	t.Run("Very small positive decimal", func(t *testing.T) {
		result, err := StringToDecimal256("0.000000000000000001", 18) // 18 decimal places
		assert.NoError(t, err)
		expected := proto.Decimal256(proto.Int256{Low: proto.UInt128{Low: 1, High: 0}, High: proto.UInt128{Low: 0, High: 0}})
		assert.Equal(t, expected, result)
	})

	// Test very small negative decimal
	t.Run("Very small negative decimal", func(t *testing.T) {
		result, err := StringToDecimal256("-0.000000000000000001", 18) // 18 decimal places
		assert.NoError(t, err)
		expected := proto.Decimal256(proto.Int256{Low: proto.UInt128{Low: ^uint64(0), High: ^uint64(0)}, High: proto.UInt128{Low: ^uint64(0), High: ^uint64(0)}})
		assert.Equal(t, expected, result)
	})

	// Test high scale for Decimal256
	t.Run("Scale 76 (max)", func(t *testing.T) {
		result, err := StringToDecimal256("0."+strings.Repeat("0", 75)+"1", 76) // 76 decimal places
		assert.NoError(t, err)
		// Very small number with max scale should work
		expected := proto.Decimal256(proto.Int256{Low: proto.UInt128{Low: 1, High: 0}, High: proto.UInt128{Low: 0, High: 0}})
		assert.Equal(t, expected, result)
	})

	// Test zero with high scale
	t.Run("Zero with scale 50", func(t *testing.T) {
		result, err := StringToDecimal256("0."+strings.Repeat("0", 50), 50) // 50 decimal places
		assert.NoError(t, err)
		expected := proto.Decimal256(proto.Int256{Low: proto.UInt128{Low: 0, High: 0}, High: proto.UInt128{Low: 0, High: 0}})
		assert.Equal(t, expected, result)
	})
}
