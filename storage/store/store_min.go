package store

import (
	"fmt"
	"math/big"
	"strconv"

	"github.com/streamingfast/substreams/bigdecimal"
)

func (b *baseStore) SetMinBigInt(ord uint64, key string, value *big.Int) {
	min := new(big.Int)
	val, found := b.GetAt(ord, key)
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
}

func (b *baseStore) SetMinInt64(ord uint64, key string, value int64) {
	var min int64
	val, found := b.GetAt(ord, key)
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
}

func (b *baseStore) SetMinFloat64(ord uint64, key string, value float64) {
	var min float64
	val, found := b.GetAt(ord, key)
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
}

func (b *baseStore) SetMinBigDecimal(ord uint64, key string, value *bigdecimal.BigDecimal) {
	min := bigdecimal.New()
	val, found := b.GetAt(ord, key)
	if !found {
		min = value
	} else {
		prev, err := bigdecimal.NewFromString(string(val))

		if err != nil || value.Cmp(prev) <= 0 {
			min = value
		} else {
			min = prev
		}
	}
	b.set(ord, key, []byte(min.String()))
}
