package entity

import (
	"database/sql/driver"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math/big"
)

type Float struct {
	float *big.Float
}

func NewFloat(f *big.Float) Float         { return Float{float: f} }
func FloatAdd(a, b Float) Float           { return Float{float: new(big.Float).Add(a.float, b.float)} }
func FloatSub(a, b Float) Float           { return Float{float: new(big.Float).Sub(a.float, b.float)} }
func FloatMul(a, b Float) Float           { return Float{float: new(big.Float).Mul(a.float, b.float)} }
func FloatQuo(a, b Float) Float           { return Float{float: new(big.Float).Quo(a.float, b.float)} }
func NewFloatFromLiteral(f float64) Float { return Float{float: big.NewFloat(f)} }

func (b *Float) Float() *big.Float              { return new(big.Float).Copy(b.float) }
func (b Float) Ptr() *Float                     { return &b }
func (b Float) String() string                  { return b.float.Text('g', -1) }
func (b Float) StringRounded(digits int) string { return b.float.Text('g', digits) }

func (b Float) MarshalJSON() ([]byte, error) {
	cnt, err := b.float.GobEncode()
	if err != nil {
		return nil, fmt.Errorf("failed to gob encode entity.Float: %w", err)
	}
	return json.Marshal(hex.EncodeToString(cnt))
}
func (b *Float) UnmarshalJSON(data []byte) error {
	var s string
	if err := json.Unmarshal(data, &s); err != nil {
		return fmt.Errorf("failed to json unmarshall entity.Float: %w", err)
	}

	cnt, err := hex.DecodeString(s)
	if err != nil {
		return fmt.Errorf("failed to hex decode entity.Float: %w", err)
	}

	v := new(big.Float)
	if err := v.GobDecode(cnt); err != nil {
		return fmt.Errorf("failed to gob decoder entity.Float: %w", err)
	}
	v.SetPrec(100)

	*b = NewFloat(v)
	return nil
}

func (b Float) MarshalCSV() ([]byte, error) {
	return []byte(b.String()), nil
}

func (b Float) Value() (driver.Value, error) {
	if b.float == nil {
		return nil, nil
	}
	// 34 is the value the `graph-node` has in its `normalized`
	// function, and corresponds to what we see in the DB for some
	// values.
	return b.float.Text('g', -1), nil
}

func (b *Float) Scan(value interface{}) error {
	if value == nil {
		return nil
	}

	sv, err := driver.String.ConvertValue(value)
	if err != nil {
		return err
	}
	bs, ok := sv.([]byte)
	if !ok {
		return fmt.Errorf("could not convert data to byte array")
	}

	newFloat, _, err := big.ParseFloat(string(bs), 10, 100, big.ToNearestEven)
	if err != nil {
		return fmt.Errorf("failed to set string %q: %s", string(bs), err)
	}

	b.float = newFloat

	return nil
}

type Int struct {
	int *big.Int
}

func NewInt(i *big.Int) Int                  { return Int{int: i} }
func IntAdd(a, b Int) Int                    { return Int{int: new(big.Int).Add(a.int, b.int)} }
func IntSub(a, b Int) Int                    { return Int{int: new(big.Int).Sub(a.int, b.int)} }
func IntMul(a, b Int) Int                    { return Int{int: new(big.Int).Mul(a.int, b.int)} }
func IntQuo(a, b Int) Int                    { return Int{int: new(big.Int).Quo(a.int, b.int)} }
func NewIntFromLiteral(i int64) Int          { return Int{int: big.NewInt(i)} }
func NewIntFromLiteralUnsigned(i uint64) Int { return Int{int: big.NewInt(0).SetUint64(i)} }

// To "copy" an Int value, an existing (or newly allocated) Int must be set to a new value
// using the Int.Set method; shallow copies of Ints are not supported and may lead to errors.
func (i *Int) Int() *big.Int { return new(big.Int).Set(i.int) }
func (i Int) Ptr() *Int      { return &i }
func (b Int) String() string { return b.int.String() }
func (b Int) AsFloat() Float { return Float{float: new(big.Float).SetInt(b.int)} }
func (b Int) MarshalJSON() ([]byte, error) {
	cnt, err := b.int.GobEncode()
	if err != nil {
		return nil, fmt.Errorf("failed to gob encode entity.Int: %w", err)
	}
	return json.Marshal(hex.EncodeToString(cnt))
}

func (b *Int) UnmarshalJSON(data []byte) error {
	var s string
	if err := json.Unmarshal(data, &s); err != nil {
		return fmt.Errorf("failed to json unmarshall entity.Float: %w", err)
	}

	cnt, err := hex.DecodeString(s)
	if err != nil {
		return fmt.Errorf("failed to hex decode entity.Float: %w", err)
	}

	v := new(big.Int)
	if err := v.GobDecode(cnt); err != nil {
		return fmt.Errorf("failed to gob decoder entity.Float: %w", err)
	}

	*b = NewInt(v)
	return nil
}

func (b Int) MarshalCSV() ([]byte, error) {
	return []byte(b.String()), nil
}

func (b Int) Value() (driver.Value, error) {
	if b.int == nil {
		return nil, nil
	}
	return b.int.String(), nil
}
func (b *Int) Scan(value interface{}) error {
	if value == nil {
		return nil
	}

	sv, err := driver.String.ConvertValue(value)
	if err != nil {
		return err
	}
	bs, ok := sv.([]byte)
	if !ok {
		return fmt.Errorf("could not convert data to byte array")
	}

	b.int = new(big.Int)
	if _, ok := b.int.SetString(string(bs), 10); !ok {
		return fmt.Errorf("failed to set string: %s", bs)
	}

	return nil
}

var zf = big.NewFloat(0)
var zi = big.NewInt(0)

func Z() Float {
	return NewFloat(zf)
}

func I() Int {
	return NewInt(zi)
}
