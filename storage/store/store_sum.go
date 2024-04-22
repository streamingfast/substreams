package store

import (
	"math/big"
	"strconv"

	"github.com/shopspring/decimal"
	pbssinternal "github.com/streamingfast/substreams/pb/sf/substreams/intern/v2"
)

func (b *baseStore) SumBigInt(ord uint64, key string, value *big.Int) {
	b.pendingOps.Add(&pbssinternal.Operation{
		Type:  pbssinternal.Operation_SUM_BIG_INT,
		Ord:   ord,
		Key:   key,
		Value: bigIntToBytes(value),
	})
}

func (b *baseStore) sumBigInt(ord uint64, key string, value *big.Int) {
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
	b.pendingOps.Add(&pbssinternal.Operation{
		Type:  pbssinternal.Operation_SUM_INT64,
		Ord:   ord,
		Key:   key,
		Value: int64ToBytes(value),
	})
}

func (b *baseStore) sumInt64(ord uint64, key string, value int64) {
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
	b.pendingOps.Add(&pbssinternal.Operation{
		Type:  pbssinternal.Operation_SUM_FLOAT64,
		Ord:   ord,
		Key:   key,
		Value: float64ToBytes(value),
	})
}

func (b *baseStore) sumFloat64(ord uint64, key string, value float64) {
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
	b.pendingOps.Add(&pbssinternal.Operation{
		Type:  pbssinternal.Operation_SUM_BIG_DECIMAL,
		Ord:   ord,
		Key:   key,
		Value: bigDecimalToBytes(value),
	})
}

func (b *baseStore) sumBigDecimal(ord uint64, key string, value decimal.Decimal) {
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
