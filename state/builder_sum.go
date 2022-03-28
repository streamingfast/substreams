package state

import (
	"fmt"
	"math/big"
	"strconv"
)

func (b *Builder) SumBigInt(ord uint64, key string, value *big.Int) error {
	sum := new(big.Int)
	val, found, err := b.GetAt(ord, key)
	if err != nil {
		return fmt.Errorf("set sum big int: %w", err)
	}

	if !found {
		sum = value
	} else {
		prev, _ := new(big.Int).SetString(string(val), 10)
		if prev == nil {
			sum = value
		} else {
			sum.Add(prev, value)
		}
	}
	b.set(ord, key, []byte(sum.String()))
	return nil
}

func (b *Builder) SumInt64(ord uint64, key string, value int64) error {
	var sum int64
	val, found, err := b.GetAt(ord, key)
	if err != nil {
		return fmt.Errorf("set sum int 64: %w", err)
	}

	if !found {
		sum = value
	} else {
		prev, err := strconv.ParseInt(string(val), 10, 64)
		if err != nil {
			sum = value
		} else {
			sum = prev + value
		}
	}
	b.set(ord, key, []byte(strconv.FormatInt(sum, 10)))
	return nil
}

func (b *Builder) SumFloat64(ord uint64, key string, value float64) error {
	var sum float64
	val, found, err := b.GetAt(ord, key)
	if err != nil {
		return fmt.Errorf("set sum big int: %w", err)
	}

	if !found {
		sum = value
	} else {
		prev, err := strconv.ParseFloat(string(val), 64)
		if err != nil {
			sum = value
		} else {
			sum = prev + value
		}
	}
	b.set(ord, key, []byte(strconv.FormatFloat(sum, 'g', 100, 64)))
	return nil
}

func (b *Builder) SumBigFloat(ord uint64, key string, value *big.Float) error {
	sum := new(big.Float)
	val, found, err := b.GetAt(ord, key)
	if err != nil {
		return fmt.Errorf("set sum big float: %w", err)
	}

	if !found {
		sum = value
	} else {
		prev, _, err := big.ParseFloat(string(val), 10, 100, big.ToNearestEven)
		if prev == nil || err != nil {
			sum = value
		} else {
			sum.Add(prev, value)
		}
	}
	b.set(ord, key, []byte(sum.Text('g', 100)))
	return nil
}
