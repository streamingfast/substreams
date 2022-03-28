package state

import (
	"fmt"
	"math/big"
	"strconv"
)

func (b *Builder) SetMaxBigInt(ord uint64, key string, value *big.Int) error {
	max := new(big.Int)
	val, found, err := b.GetAt(ord, key)
	if err != nil {
		return fmt.Errorf("set max big int: %w", err)
	}
	if !found {
		max = value
	} else {
		prev, _ := new(big.Int).SetString(string(val), 10)
		if prev != nil && value.Cmp(prev) > 0 {
			max = value
		} else {
			max = prev
		}
	}
	b.set(ord, key, []byte(max.String()))
	return nil
}

func (b *Builder) SetMaxInt64(ord uint64, key string, value int64) error {
	var max int64
	val, found, err := b.GetAt(ord, key)
	if err != nil {
		return fmt.Errorf("set max int 64: %w", err)
	}
	if !found {
		max = value
	} else {
		prev, err := strconv.ParseInt(string(val), 10, 64)
		if err != nil || value > prev {
			max = value
		} else {
			max = prev
		}
	}
	b.set(ord, key, []byte(fmt.Sprintf("%d", max)))
	return nil
}

func (b *Builder) SetMaxFloat64(ord uint64, key string, value float64) error {
	var max float64
	val, found, err := b.GetAt(ord, key)
	if err != nil {
		return fmt.Errorf("set max float 64: %w", err)
	}

	if !found {
		max = value
	} else {
		prev, err := strconv.ParseFloat(string(val), 64)

		if err != nil || value > prev {
			max = value
		} else {
			max = prev
		}
	}
	b.set(ord, key, []byte(strconv.FormatFloat(max, 'g', 100, 64)))
	return nil
}

func (b *Builder) SetMaxBigFloat(ord uint64, key string, value *big.Float) error {
	max := new(big.Float)
	val, found, err := b.GetAt(ord, key)
	if err != nil {
		return fmt.Errorf("set max big float: %w", err)
	}

	if !found {
		max = value
	} else {
		prev, _, err := big.ParseFloat(string(val), 10, 100, big.ToNearestEven)

		if err != nil || value.Cmp(prev) > 0 {
			max = value
		} else {
			max = prev
		}
	}
	b.set(ord, key, []byte(max.Text('g', -1)))
	return nil
}
