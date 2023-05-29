package bigdecimal

import (
	"fmt"
	"math/big"
	"strconv"
	"strings"
)

// Max signigicant digits accepted by `graph-node`
//
// See https://github.com/graphprotocol/graph-node/blob/9d013f75f2a565e3d126737593e3a30d1b2f212e/graph/src/data/store/scalar.rs#L46
const MAX_SIGNIFICANT_DIGITS = uint64(34)

// A few BigDecimal constants for comparisions purposes only!
//
// Pay great care when you use those, you must **never** use those
// to construct the `Int` part of a BigDecimal. Indeed, BigDecimal
// has a few `InPlace` operation that would mutate the `Int` part
// which would mutates those constants if they were to be used when
// constructing a BigDecimal.
var (
	bigZero = big.NewInt(0)
	bigOne  = big.NewInt(1)
	bigTwo  = big.NewInt(2)
	bigFive = big.NewInt(5)
	bigTen  = big.NewInt(10)
)

// BigDecimal replicates `graph-node` way of representing, parsing and printing
// big decimal values.
//
// The goal of this type is not to treat number just like `graph-node` would do it
// for deterministic stable hashing purposes.
//
// This type is essential a port of https://github.com/akubera/bigdecimal-rs/tree/v0.1.2
// as well as some code from https://github.com/graphprotocol/graph-node/blob/9d013f75f2a565e3d126737593e3a30d1b2f212e/graph/src/data/store/scalar.rs#L74.
//
// The NewBigDecimalFromString implements parsing of the string representation and applying
// `graph-node` `normalized` rules.
type BigDecimal struct {
	Int   *big.Int
	Scale int64
}

// New creates a new BigDecimal sets to 0.
func New() *BigDecimal {
	return &BigDecimal{&big.Int{}, 0}
}

// Zero creates a new BigDecimal sets to 0.
func Zero() *BigDecimal {
	return &BigDecimal{&big.Int{}, 0}
}

// MustNewFromString is like NewFromString but panics on error.
func MustNewFromString(s string) *BigDecimal {
	out, err := NewFromString(s)
	if err != nil {
		panic(err)
	}

	return out
}

// NewFromString creates a new BigDecimal from a string representation, essentially
// the parse from string operation.
func NewFromString(s string) (*BigDecimal, error) {
	basePart, exponentValue := s, int64(0)
	if loc := strings.IndexAny(s, "eE"); loc != -1 {
		// let (base, exp) = (&s[..loc], &s[loc + 1..]);
		//
		// // special consideration for rust 1.0.0 which would not parse a leading '+'
		//let exp = match exp.chars().next() {
		// 	Some('+') => &exp[1..],
		// 	_ => exp,
		// };
		// slice up to `loc` and 1 after to skip the 'e' char
		base, expRaw := s[:loc], strings.TrimPrefix(s[loc+1:], "+")

		exp, err := strconv.ParseInt(expRaw, 0, 64)
		if err != nil {
			return nil, fmt.Errorf("invalid exponent value %q: %w", expRaw, err)
		}

		basePart = base
		exponentValue = exp
	}

	if basePart == "" {
		return nil, fmt.Errorf("failed to parse empty string")
	}

	digits, decimalOffset := basePart, int64(0)
	if loc := strings.IndexAny(s, "."); loc != -1 {
		// let (lead, trail) = (&base_part[..loc], &base_part[loc + 1..]);
		lead, trail := basePart[:loc], basePart[loc+1:]

		// let mut digits = String::from(lead);
		// digits.push_str(trail);
		// copy leading characters + trailing characters after '.' into the digits string
		digits = lead + trail
		decimalOffset = int64(len(trail))
	}

	// let scale = decimal_offset - exponent_value;
	// let big_int = try!(BigInt::from_str_radix(&digits, radix));
	scale := decimalOffset - exponentValue
	bigInt, ok := (&big.Int{}).SetString(digits, 10)
	if !ok {
		return nil, fmt.Errorf("invalid digits part %q", digits)
	}

	out := &BigDecimal{Int: bigInt, Scale: scale}
	out.normalizeInPlace()

	return out, nil
}

func (z *BigDecimal) String() string {
	//   // Aquire the absolute integer as a decimal string
	//   let mut absInt = self.int_val.abs().to_str_radix(10);
	// It's more efficient to do String and remove the '-' to make it absolute than to use (&big.Int{}).Abs.Text(10)
	absInt := strings.TrimPrefix(z.Int.Text(10), "-")

	// Split the representation at the decimal point
	//   let (before, after) = if self.scale >= abs_int.len() as i64 {
	var before, after string
	if z.Scale >= int64(len(absInt)) {
		// First case: the integer representation falls completely behind the decimal point
		//     let scale = self.scale as usize;
		//     let after = "0".repeat(scale - abs_int.len()) + abs_int.as_str();
		//     ("0".to_string(), after)
		before, after = "0", strings.Repeat("0", int(z.Scale)-len(absInt))+absInt
	} else {
		// Second case: the integer representation falls around, or before the decimal point
		//     let location = abs_int.len() as i64 - self.scale;
		//     if location > abs_int.len() as i64 {
		location := int64(len(absInt)) - z.Scale
		if location > int64(len(absInt)) {
			// Case 2.1, entirely before the decimal point, we should prepend zeros
			//   let zeros = location as usize - abs_int.len();
			//   let abs_int = abs_int + "0".repeat(zeros as usize).as_str();
			//   (abs_int, "".to_string())
			zeros := location - int64(len(absInt))
			before, after = absInt+strings.Repeat("0", int(zeros)), ""
		} else {
			// Case 2.2, somewhere around the decimal point, just split it in two
			// 		  let after = abs_int.split_off(location as usize);
			// 		  (abs_int, after)
			before, after = absInt[0:location], absInt[location:]
		}
	}

	// Concatenate everything
	//   let complete_without_sign = if !after.is_empty() { before + "." + after.as_str() else { before };
	completeWithoutSign := before
	if after != "" {
		completeWithoutSign = before + "." + after
	}

	// If negative, prepend a minus sign
	if z.Int.Sign() == -1 {
		return "-" + completeWithoutSign
	}

	return completeWithoutSign
}

func (z *BigDecimal) IsZero() bool {
	// The `Sign` calls on big.Int returns 0 if number is equal 0 (-1 or 1 otherwise)
	return z.Scale == 0 && z.Int.Sign() == 0
}

func (z *BigDecimal) Add(left *BigDecimal, right *BigDecimal) *BigDecimal {
	switch {
	// 	Ordering::Equal => { lhs.int_val += rhs.int_val; return lhs }
	case left.Scale == right.Scale:
		z.Int.Add(left.Int, right.Int)
		z.Scale = left.Scale
		return z

	// Ordering::Less => lhs.take_and_scale(rhs.scale) + rhs,
	case left.Scale < right.Scale:
		z = z.takeAndScale(left, right.Scale)
		// Rust was recursive here, but we instead aligned the == scale above
		z.Int = z.Int.Add(z.Int, right.Int)

		return z

	// Ordering::Greater => rhs.take_and_scale(lhs.scale) + lhs,
	case left.Scale > right.Scale:
		z = z.takeAndScale(right, left.Scale)
		// Rust was recursive here, but we instead aligned the == scale above
		z.Int = z.Int.Add(z.Int, left.Int)

		return z

	default:
		panic("unreachable, we cover all cases (==, <, >)")
	}
}

func (z *BigDecimal) takeAndScale(x *BigDecimal, newScale int64) *BigDecimal {
	// if self.int_val.is_zero() {
	// 	return BigDecimal::new(BigInt::zero(), new_scale);
	// }
	if x.Int.Sign() == 0 {
		*z.Int = *bigZero
		z.Scale = newScale

		return z
	}

	// if new_scale > self.scale {
	// 	self.int_val *= ten_to_the((new_scale - self.scale) as u64);
	// 	BigDecimal::new(self.int_val, new_scale)
	// }
	if newScale > x.Scale {
		z.Int.Mul(x.Int, tenToThe(uint64(newScale-x.Scale)))
		z.Scale = newScale

		return z
	}

	// if new_scale < self.scale {
	// 	self.int_val /= ten_to_the((self.scale - new_scale) as u64);
	// 	BigDecimal::new(self.int_val, new_scale)
	// }
	if newScale < x.Scale {
		z.Int.Quo(x.Int, tenToThe(uint64(x.Scale-newScale)))
		z.Scale = newScale

		return z
	}

	*z = *x
	return z
}

func (z *BigDecimal) normalizeInPlace() {
	if z.IsZero() {
		return
	}

	// Round to the maximum significant digits.
	z.withPrecisionInPlace(MAX_SIGNIFICANT_DIGITS)

	// let (bigint, exp) = big_decimal.as_bigint_and_exponent();
	bigint, exp := z.Int, z.Scale
	trace("normalized: as_bigint_and_exponent (bigint %s, exp %d)", bigint, exp)

	// let (sign, mut digits) = bigint.to_radix_be(10);
	sign, digits := bigint.Sign(), bigint.Abs(bigint).String()
	trace("normalized: to_radix_be (sign %s, digits (str) %s)", Sign(sign), digits)

	// let trailing_count = digits.iter().rev().take_while(|i| **i == 0).count();
	// digits.truncate(digits.len() - trailing_count);
	digits, trailingCount := trailingZeroTruncated(digits)
	trace("normalized: trailing_count %d", trailingCount)
	trace("normalized: digits truncated %s", digits)

	// let int_val = num_bigint::BigInt::from_radix_be(sign, &digits, 10).unwrap();
	z.Int, _ = (&big.Int{}).SetString(digits, 10)
	if z.Int == nil {
		z.Int = big.NewInt(0)
	}
	if sign == -1 {
		z.Int = z.Int.Neg(z.Int)
	}
	trace("normalized: int_val %s", z.Int)

	// let scale = exp - trailing_count as i64;
	z.Scale = exp - trailingCount
	trace("normalized: scale %d", z.Scale)
	// BigDecimal(bigdecimal::BigDecimal::new(int_val, scale))
}

func trailingZeroTruncated(in string) (string, int64) {
	out := strings.TrimRight(in, "0")
	return out, int64(len(in) - len(out))
}

func (z *BigDecimal) withPrecisionInPlace(prec uint64) {
	digits := z.digits()
	trace("with_prec: digits %d", digits)

	if digits > prec {
		trace("with_prec: digits > prec")

		diff := digits - prec
		p := tenToThe(diff)

		var q *big.Int
		// let (mut q, r) = self.int_val.div_rem(&p);
		q, r := (&big.Int{}).QuoRem(z.Int, p, &big.Int{})
		trace("with_prec: digits > prec (q %s, r %s)", q, r)

		// check for "leading zero" in remainder term; otherwise round
		tenTimesR := (&big.Int{}).Mul(bigTen, r)
		if p.Cmp(tenTimesR) == -1 {
			roundingTerm := getRoundingTerm(r)
			q = q.Add(q, roundingTerm)
			trace("with_prec: digits > prec adding rounding term %s", roundingTerm)
		}

		z.Int = q
		z.Scale = z.Scale - int64(diff)
		trace("with_prec: digits > prec got (bigint %s, exp %d)", z.Int, z.Scale)

		return
	}

	if digits < prec {
		trace("with_prec: digits < prec")

		diff := prec - digits
		p := tenToThe(diff)

		z.Int = z.Int.Mul(z.Int, p)
		z.Scale = z.Scale + int64(diff)
		trace("with_prec: digits < prec got (bigint %s, exp %d)", z.Int, z.Scale)

		return
	}

	trace("with_prec: digits == prec")
}

// Digits gives number of digits in the non-scaled integer representation
func (b *BigDecimal) digits() uint64 {
	bInt := b.Int
	if bInt.Sign() == 0 {
		return 1
	}

	// guess number of digits based on number of bits in UInt
	// let mut digits = (int.bits() as f64 / 3.3219280949) as u64;
	bits := uint(bInt.BitLen())
	trace("digits: bits %d", bits)

	digits := uint64(float64(bits) / 3.3219280949)
	trace("digits: guess digits %d", digits)

	// let mut num = ten_to_the(digits);
	num := (&big.Int{}).Set(tenToThe(digits))
	trace("digits: num %s", num)

	// while int >= &num {
	// 	num *= 10u8;
	// 	digits += 1;
	// }
	for bInt.Cmp(num) >= 0 {
		num = num.Mul(num, bigTen)
		digits += 1
		trace("digits: add one digit")
	}

	trace("digits: final digits %d", digits)
	return digits
}

var tenToPrecomputeTable []*big.Int

func init() {
	tenToPrecomputeTable = make([]*big.Int, 35+1)
	for i := 0; i <= 35; i++ {
		tenToPrecomputeTable[i] = (&big.Int{}).Exp(bigTen, big.NewInt(int64(i)), nil)
	}
}

func tenToThe(pow uint64) *big.Int {
	if pow < uint64(len(tenToPrecomputeTable)) {
		return tenToPrecomputeTable[pow]
	}

	return (&big.Int{}).Exp(bigTen, big.NewInt(int64(pow)), nil)
}

func getRoundingTerm(num *big.Int) *big.Int {
	if num.Sign() == 0 {
		return bigZero
	}

	// let digits = (num.bits() as f64 / 3.3219280949) as u64;
	bits := uint(num.BitLen()) - num.TrailingZeroBits()
	digits := uint64(float64(bits) / 3.3219280949)

	// let mut n = ten_to_the(digits);
	n := (&big.Int{}).Set(tenToThe(digits))

	// loop-method
	for {
		if num.Cmp(n) == -1 {
			return bigOne
		}

		n = n.Mul(n, bigFive)
		if num.Cmp(n) == -1 {
			return bigZero
		}

		n = n.Mul(n, bigTwo)
	}

	// string-method
	// let s = format!("{}", num);
	// let high_digit = u8::from_str(&s[0..1]).unwrap();
	// if high_digit < 5 { 0 } else { 1 }
}

type Sign int

func (s Sign) String() string {
	if s <= -1 {
		return "SignMinus"
	}

	if s >= 1 {
		return "SignPlus"
	}

	return "NoSign"
}

// DEBUG_BIGDECIMAL the logging tracer is so heaviy if activated by default that it's worth
// putting all the tracing support behind a manually activated flag.
//
// **Important** Don't forget to set it back to false once you have debugged enough
const DEBUG_BIGDECIMAL = false

// trace traces the following print statement through `zlog` logger if [DEBUG_BIGDECIMAL]
// in-code static variable is set to `true` (needs to be manually changed and program re-compiled
// to have an effect) and if `tracer` is enabled.
func trace(msg string, args ...any) {
	if DEBUG_BIGDECIMAL && tracer.Enabled() {
		zlog.Debug(fmt.Sprintf(msg, args...))
	}
}
