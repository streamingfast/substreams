package store

import (
	"math/big"
	"strconv"

	"github.com/shopspring/decimal"
)

func (b *baseStore) SumBigInt(ord uint64, key string, value *big.Int) {
	sum := new(big.Int)
	val, found := b.GetAt(ord, key)
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
}

func (b *baseStore) SumInt64(ord uint64, key string, value int64) {
	var sum int64
	val, found := b.GetAt(ord, key)
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
}

func (b *baseStore) SumFloat64(ord uint64, key string, value float64) {
	var sum float64
	val, found := b.GetAt(ord, key)
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
}

func (b *baseStore) SumBigDecimal(ord uint64, key string, value decimal.Decimal) {
	v, found := b.GetAt(ord, key)
	if !found {
		b.set(ord, key, []byte(value.String()))
		return
	}
	prev, err := decimal.NewFromString(string(v))
	prev.Truncate(34)
	if err != nil {
		b.set(ord, key, []byte(value.String()))
		return
	}
	sum := prev.Add(value)
	b.set(ord, key, []byte(sum.String()))
}
