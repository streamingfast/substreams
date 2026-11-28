package rows

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCheckBigInt(t *testing.T) {
	tests := []struct {
		name   string
		in     string
		signed bool
		bits   uint16
		want   string // error substring, empty when valid
	}{
		{"empty is zero", "", false, 128, ""},
		{"zero", "0", false, 128, ""},
		{"digits", "340282366920938463463374607431768211455", false, 128, ""},
		{"leading zeros", "000012", false, 128, ""},
		{"signed negative", "-12", true, 128, ""},
		{"negative zero", "-0", true, 256, ""},
		{"unsigned negative", "-1", false, 128, "negative value in an unsigned column"},
		{"only a sign", "-", true, 128, "no digits"},
		{"plus sign", "+1", true, 128, "unexpected character"},
		{"space", " 1", true, 128, "unexpected character"},
		{"hex", "0x10", true, 128, "unexpected character"},
		{"underscore", "1_000", true, 128, "unexpected character"},
		{"decimal point", "1.5", true, 128, "unexpected character"},
		{"too many digits for 128", strings.Repeat("9", 40), false, 128, "do not fit in 128 bits"},
		{"39 digits for 128", strings.Repeat("9", 39), false, 128, ""},
		{"78 digits for 256", strings.Repeat("9", 78), false, 256, ""},
		{"too many digits for 256", strings.Repeat("9", 79), false, 256, "do not fit in 256 bits"},
		{"leading zeros do not count", strings.Repeat("0", 50) + "1", false, 128, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := CheckBigInt(tt.in, tt.signed, tt.bits)
			if tt.want == "" {
				require.NoError(t, err)
				return
			}
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.want)
		})
	}
}

func TestCheckDecimal(t *testing.T) {
	tests := []struct {
		name  string
		in    string
		scale int32
		want  string
	}{
		{"empty is zero", "", 4, ""},
		{"integer", "12", 4, ""},
		{"fraction", "12.3456", 4, ""},
		{"fewer fractional digits", "12.3", 4, ""},
		{"negative", "-0.5", 4, ""},
		{"too many fractional digits", "1.23456", 4, "5 fractional digits, scale is 4"},
		{"trailing point", "12.", 4, "no digits after the decimal point"},
		{"leading point", ".5", 4, "no digits before the decimal point"},
		{"two points", "1.2.3", 4, "unexpected character"},
		{"only a sign", "-", 4, "no digits"},
		{"letters", "1.2x", 4, "unexpected character"},
		{"too many integer digits", strings.Repeat("9", 40) + ".1", 4, "do not fit"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := CheckDecimal(tt.in, 128, tt.scale)
			if tt.want == "" {
				require.NoError(t, err)
				return
			}
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.want)
		})
	}
}

func TestChecks_DoNotAllocate(t *testing.T) {
	assert.Zero(t, testing.AllocsPerRun(100, func() {
		_ = CheckBigInt("115792089237316195423570985008687907853269984665640564039457584007913129639935", false, 256)
		_ = CheckDecimal("12.3456", 128, 4)
	}))
}
