package store

import (
	"fmt"
	"math/big"
	"strconv"

	"github.com/shopspring/decimal"
	pbssinternal "github.com/streamingfast/substreams/pb/sf/substreams/intern/v2"
)

func (b *baseStore) SetMinBigInt(ord uint64, key string, value *big.Int) {
	b.pendingOps.Add(&pbssinternal.Operation{
		Type:  pbssinternal.Operation_SET_MIN_BIG_INT,
		Ord:   ord,
		Key:   key,
		Value: bigIntToBytes(value),
	})
}

func (b *baseStore) setMinBigInt(ord uint64, key string, value *big.Int) {
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
	b.pendingOps.Add(&pbssinternal.Operation{
		Type:  pbssinternal.Operation_SET_MIN_INT64,
		Ord:   ord,
		Key:   key,
		Value: int64ToBytes(value),
	})
}

func (b *baseStore) setMinInt64(ord uint64, key string, value int64) {
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
	b.pendingOps.Add(&pbssinternal.Operation{
		Type:  pbssinternal.Operation_SET_MIN_FLOAT64,
		Ord:   ord,
		Key:   key,
		Value: float64ToBytes(value),
	})
}

func (b *baseStore) setMinFloat64(ord uint64, key string, value float64) {
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

func (b *baseStore) SetMinBigDecimal(ord uint64, key string, value decimal.Decimal) {
	b.pendingOps.Add(&pbssinternal.Operation{
		Type:  pbssinternal.Operation_SET_MIN_BIG_DECIMAL,
		Ord:   ord,
		Key:   key,
		Value: bigDecimalToBytes(value),
	})
}

func (b *baseStore) setMinBigDecimal(ord uint64, key string, value decimal.Decimal) {
	val, found := b.GetAt(ord, key)
	if !found {
		b.set(ord, key, []byte(value.String()))
		return
	}
	prev, err := decimal.NewFromString(string(val))
	prev.Truncate(34)
	if err != nil || value.Cmp(prev) <= 0 {
		b.set(ord, key, []byte(value.String()))
		return
	}
	b.set(ord, key, []byte(prev.String()))
}
