package state

import (
	"fmt"
	"math/big"
	"strconv"
)

func (b *Builder) SetMinBigInt(ord uint64, key string, value *big.Int) error {
	min := new(big.Int)
	val, found, err := b.GetAt(ord, key)
	if err != nil {
		return fmt.Errorf("set min big int: %w", err)
	}

	if !found {
		min = value
	} else {
		prev, _ := new(big.Int).SetString(string(val), 10)
		if prev != nil && value.Cmp(prev) <= 0 {
			min = value
		} else {
			min = prev
		}
	}
	b.set(ord, key, []byte(min.String()))
	return nil
}

func (b *Builder) SetMinInt64(ord uint64, key string, value int64) error {
	var min int64
	val, found, err := b.GetAt(ord, key)
	if err != nil {
		return fmt.Errorf("set min big 64: %w", err)
	}

	if !found {
		min = value
	} else {
		prev, err := strconv.ParseInt(string(val), 10, 64)
		if err != nil || value < prev {
			min = value
		} else {
			min = prev
		}
	}
	b.set(ord, key, []byte(fmt.Sprintf("%d", min)))
	return nil
}

func (b *Builder) SetMinFloat64(ord uint64, key string, value float64) error {
	var min float64
	val, found, err := b.GetAt(ord, key)
	if err != nil {
		return fmt.Errorf("set min float 64: %w", err)
	}

	if !found {
		min = value
	} else {
		prev, err := strconv.ParseFloat(string(val), 64)

		if err != nil || value <= prev {
			min = value
		} else {
			min = prev
		}
	}
	b.set(ord, key, []byte(strconv.FormatFloat(min, 'g', 100, 64)))
	return nil
}

func (b *Builder) SetMinBigFloat(ord uint64, key string, value *big.Float) error {
	min := new(big.Float)
	val, found, err := b.GetAt(ord, key)
	if err != nil {
		return fmt.Errorf("set min big float: %w", err)
	}

	if !found {
		min = value
	} else {
		prev, _, err := big.ParseFloat(string(val), 10, 100, big.ToNearestEven)

		if err != nil || value.Cmp(prev) <= 0 {
			min = value
		} else {
			min = prev
		}
	}
	b.set(ord, key, []byte(min.Text('g', -1)))
	return nil
}
