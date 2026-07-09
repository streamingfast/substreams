package clickhouse

import (
	"testing"

	"github.com/ClickHouse/ch-go/proto"
	"github.com/stretchr/testify/assert"
)

func TestStringToInt128(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		expected  proto.Int128
		expectErr bool
		errMsg    string
	}{
		// Valid positive cases
		{
			name:     "Simple positive integer",
			input:    "12345",
			expected: proto.Int128{Low: 12345, High: 0},
		},
		{
			name:     "Zero",
			input:    "0",
			expected: proto.Int128{Low: 0, High: 0},
		},
		{
			name:     "Large positive number",
			input:    "18446744073709551615", // max uint64
			expected: proto.Int128{Low: 18446744073709551615, High: 0},
		},
		{
			name:     "Very large positive number requiring high bits",
			input:    "170141183460469231731687303715884105727", // 2^127 - 1 (max for signed Int128)
			expected: proto.Int128{Low: 18446744073709551615, High: 9223372036854775807},
		},
		{
			name:     "Positive with leading whitespace",
			input:    "  12345",
			expected: proto.Int128{Low: 12345, High: 0},
		},
		{
			name:     "Positive with trailing whitespace",
			input:    "12345  ",
			expected: proto.Int128{Low: 12345, High: 0},
		},

		// Valid negative cases
		{
			name:     "Simple negative integer",
			input:    "-12345",
			expected: proto.Int128{Low: ^uint64(12344), High: ^uint64(0)}, // Two's complement
		},
		{
			name:     "Negative one",
			input:    "-1",
			expected: proto.Int128{Low: ^uint64(0), High: ^uint64(0)}, // All bits set
		},
		{
			name:     "Large negative number",
			input:    "-170141183460469231731687303715884105728",      // -2^127
			expected: proto.Int128{Low: 0, High: 9223372036854775808}, // Sign bit set in high
		},

		// Error cases
		{
			name:      "Empty string",
			input:     "",
			expectErr: true,
			errMsg:    "empty string cannot be converted to int128",
		},
		{
			name:      "Only whitespace",
			input:     "   ",
			expectErr: true,
			errMsg:    "empty string cannot be converted to int128",
		},
		{
			name:      "Invalid format - letters",
			input:     "abc123",
			expectErr: true,
			errMsg:    "invalid integer format: abc123",
		},
		{
			name:      "Invalid format - mixed",
			input:     "123abc",
			expectErr: true,
			errMsg:    "invalid integer format: 123abc",
		},
		{
			name:      "Too large positive",
			input:     "170141183460469231731687303715884105728", // 2^127
			expectErr: true,
			errMsg:    "integer value out of range for Int128",
		},
		{
			name:      "Too large negative",
			input:     "-170141183460469231731687303715884105729", // -2^127 - 1
			expectErr: true,
			errMsg:    "integer value out of range for Int128",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := StringToInt128(tt.input)

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

func TestStringToUInt128(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		expected  proto.UInt128
		expectErr bool
		errMsg    string
	}{
		// Valid cases
		{
			name:     "Simple positive integer",
			input:    "12345",
			expected: proto.UInt128{Low: 12345, High: 0},
		},
		{
			name:     "Zero",
			input:    "0",
			expected: proto.UInt128{Low: 0, High: 0},
		},
		{
			name:     "Large positive number",
			input:    "18446744073709551615", // max uint64
			expected: proto.UInt128{Low: 18446744073709551615, High: 0},
		},
		{
			name:     "Max UInt128",
			input:    "340282366920938463463374607431768211455", // 2^128 - 1
			expected: proto.UInt128{Low: 18446744073709551615, High: 18446744073709551615},
		},
		{
			name:     "Positive with leading plus sign",
			input:    "+12345",
			expected: proto.UInt128{Low: 12345, High: 0},
		},
		{
			name:     "Positive with whitespace",
			input:    "  12345  ",
			expected: proto.UInt128{Low: 12345, High: 0},
		},

		// Error cases
		{
			name:      "Empty string",
			input:     "",
			expectErr: true,
			errMsg:    "empty string cannot be converted to uint128",
		},
		{
			name:      "Negative number",
			input:     "-12345",
			expectErr: true,
			errMsg:    "negative values not allowed for UInt128",
		},
		{
			name:      "Invalid format",
			input:     "abc123",
			expectErr: true,
			errMsg:    "invalid integer format: abc123",
		},
		{
			name:      "Too large",
			input:     "340282366920938463463374607431768211456", // 2^128
			expectErr: true,
			errMsg:    "integer value out of range for UInt128",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := StringToUInt128(tt.input)

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

func TestStringToInt256(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		expected  proto.Int256
		expectErr bool
		errMsg    string
	}{
		// Valid positive cases
		{
			name:     "Simple positive integer",
			input:    "12345",
			expected: proto.Int256{Low: proto.UInt128{Low: 12345, High: 0}, High: proto.UInt128{Low: 0, High: 0}},
		},
		{
			name:     "Zero",
			input:    "0",
			expected: proto.Int256{Low: proto.UInt128{Low: 0, High: 0}, High: proto.UInt128{Low: 0, High: 0}},
		},
		{
			name:     "Large positive number",
			input:    "123456789012345678901234567890",
			expected: proto.Int256{Low: proto.UInt128{Low: 14083847773837265618, High: 6692605942}, High: proto.UInt128{Low: 0, High: 0}},
		},
		{
			name:     "Very large positive number",
			input:    "57896044618658097711785492504343953926634992332820282019728792003956564819967", // 2^255 - 1 (max Int256)
			expected: proto.Int256{Low: proto.UInt128{Low: 18446744073709551615, High: 18446744073709551615}, High: proto.UInt128{Low: 18446744073709551615, High: 9223372036854775807}},
		},

		// Valid negative cases
		{
			name:     "Simple negative integer",
			input:    "-12345",
			expected: proto.Int256{Low: proto.UInt128{Low: ^uint64(12344), High: ^uint64(0)}, High: proto.UInt128{Low: ^uint64(0), High: ^uint64(0)}},
		},
		{
			name:     "Negative one",
			input:    "-1",
			expected: proto.Int256{Low: proto.UInt128{Low: ^uint64(0), High: ^uint64(0)}, High: proto.UInt128{Low: ^uint64(0), High: ^uint64(0)}},
		},

		// Error cases
		{
			name:      "Empty string",
			input:     "",
			expectErr: true,
			errMsg:    "empty string cannot be converted to int256",
		},
		{
			name:      "Invalid format",
			input:     "abc123",
			expectErr: true,
			errMsg:    "invalid integer format: abc123",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := StringToInt256(tt.input)

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

func TestStringToUInt256(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		expected  proto.UInt256
		expectErr bool
		errMsg    string
	}{
		// Valid cases
		{
			name:     "Simple positive integer",
			input:    "12345",
			expected: proto.UInt256{Low: proto.UInt128{Low: 12345, High: 0}, High: proto.UInt128{Low: 0, High: 0}},
		},
		{
			name:     "Zero",
			input:    "0",
			expected: proto.UInt256{Low: proto.UInt128{Low: 0, High: 0}, High: proto.UInt128{Low: 0, High: 0}},
		},
		{
			name:     "Large positive number",
			input:    "123456789012345678901234567890",
			expected: proto.UInt256{Low: proto.UInt128{Low: 14083847773837265618, High: 6692605942}, High: proto.UInt128{Low: 0, High: 0}},
		},
		{
			name:     "Positive with plus sign",
			input:    "+12345",
			expected: proto.UInt256{Low: proto.UInt128{Low: 12345, High: 0}, High: proto.UInt128{Low: 0, High: 0}},
		},

		// Error cases
		{
			name:      "Empty string",
			input:     "",
			expectErr: true,
			errMsg:    "empty string cannot be converted to uint256",
		},
		{
			name:      "Negative number",
			input:     "-12345",
			expectErr: true,
			errMsg:    "negative values not allowed for UInt256",
		},
		{
			name:      "Invalid format",
			input:     "abc123",
			expectErr: true,
			errMsg:    "invalid integer format: abc123",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := StringToUInt256(tt.input)

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

func TestStringToInt128_EdgeCases(t *testing.T) {
	// Test boundary values
	t.Run("Max positive Int128", func(t *testing.T) {
		result, err := StringToInt128("170141183460469231731687303715884105727") // 2^127 - 1
		assert.NoError(t, err)
		expected := proto.Int128{Low: 18446744073709551615, High: 9223372036854775807}
		assert.Equal(t, expected, result)
	})

	t.Run("Min negative Int128", func(t *testing.T) {
		result, err := StringToInt128("-170141183460469231731687303715884105728") // -2^127
		assert.NoError(t, err)
		expected := proto.Int128{Low: 0, High: 9223372036854775808}
		assert.Equal(t, expected, result)
	})
}

func TestStringToUInt128_EdgeCases(t *testing.T) {
	// Test boundary values
	t.Run("Max UInt128", func(t *testing.T) {
		result, err := StringToUInt128("340282366920938463463374607431768211455") // 2^128 - 1
		assert.NoError(t, err)
		expected := proto.UInt128{Low: 18446744073709551615, High: 18446744073709551615}
		assert.Equal(t, expected, result)
	})

	t.Run("One", func(t *testing.T) {
		result, err := StringToUInt128("1")
		assert.NoError(t, err)
		expected := proto.UInt128{Low: 1, High: 0}
		assert.Equal(t, expected, result)
	})
}
