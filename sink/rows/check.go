package rows

import (
	"errors"
	"fmt"
	"strings"
)

// checkKind says which validation a column needs when [WithCheck] is on.
type checkKind uint8

const (
	checkNone checkKind = iota
	checkBigInt
	checkDecimal
)

// ValidationError is returned by the walk when [WithCheck] is on and a bigint or decimal value is malformed.
type ValidationError struct {
	Block  BlockRef
	Table  string
	Column string
	// Value is the offending value, truncated.
	Value  string
	Reason string
}

func (e *ValidationError) Error() string {
	return fmt.Sprintf("block %d: invalid value %q for column %s.%s: %s", e.Block.Number, e.Value, e.Table, e.Column, e.Reason)
}

// Unwrap lets callers match errors.Is(err, ErrInvalidValue).
func (e *ValidationError) Unwrap() error { return ErrInvalidValue }

// ErrInvalidValue matches every [ValidationError].
var ErrInvalidValue = errors.New("invalid value")

func truncate(s string) string {
	if len(s) > 64 {
		return s[:64]
	}
	return s
}

// maxDigits bounds the integer digits of a 128-bit and a 256-bit number. It is a necessary, not a sufficient, range
// check: the exact bit range is the consumer's job.
func maxDigits(bits uint16) int {
	if bits == 256 {
		return 78
	}
	return 39
}

// CheckBigInt validates the text of a bigint column: an optional `-` (only when signed), at least one digit, no sign
// `+`, no spaces or separators, and no more integer digits than fit in bits. The empty string is valid and means zero.
// It does not allocate.
func CheckBigInt(s string, signed bool, bits uint16) error {
	if s == "" {
		return nil
	}

	digits := s
	if s[0] == '-' {
		if !signed {
			return errors.New("negative value in an unsigned column")
		}
		digits = s[1:]
	}
	if digits == "" {
		return errors.New("no digits")
	}

	return checkDigits(digits, bits)
}

// CheckDecimal validates the text of a decimal column: an optional `-`, digits, and when there is a `.`, at least one and at
// most scale fractional digits. The empty string is valid and means zero. It does not allocate.
func CheckDecimal(s string, bits uint16, scale int32) error {
	if s == "" {
		return nil
	}

	if s[0] == '-' {
		s = s[1:]
	}
	if s == "" {
		return errors.New("no digits")
	}

	whole, fraction := s, ""
	if i := strings.IndexByte(s, '.'); i >= 0 {
		whole, fraction = s[:i], s[i+1:]
		if fraction == "" {
			return errors.New("no digits after the decimal point")
		}
		if int32(len(fraction)) > scale {
			return fmt.Errorf("%d fractional digits, scale is %d", len(fraction), scale)
		}
		for i := range len(fraction) {
			if fraction[i] < '0' || fraction[i] > '9' {
				return fmt.Errorf("unexpected character %q", fraction[i])
			}
		}
	}
	if whole == "" {
		return errors.New("no digits before the decimal point")
	}

	return checkDigits(whole, bits)
}

func checkDigits(digits string, bits uint16) error {
	for i := range len(digits) {
		if digits[i] < '0' || digits[i] > '9' {
			return fmt.Errorf("unexpected character %q", digits[i])
		}
	}

	significant := len(strings.TrimLeft(digits, "0"))
	if significant > maxDigits(bits) {
		return fmt.Errorf("%d digits do not fit in %d bits", significant, bits)
	}
	return nil
}

func checkScalar(kind checkKind, column *Column, s string) error {
	switch kind {
	case checkBigInt:
		return CheckBigInt(s, column.Type.Signed, column.Type.Bits)
	case checkDecimal:
		return CheckDecimal(s, column.Type.Bits, column.Type.Scale)
	}
	return nil
}
