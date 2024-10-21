package store

import (
	"math/big"
	"strconv"

	"github.com/shopspring/decimal"

	pbssinternal "github.com/streamingfast/substreams/pb/sf/substreams/intern/v2"
)

func (b *baseStore) SetSumInt64(ord uint64, key string, value []byte) {
	b.kvOps.Add(&pbssinternal.Operation{
		Type:  pbssinternal.Operation_SET_SUM_INT64,
		Ord:   ord,
		Key:   key,
		Value: value,
	})
}

func (b *baseStore) setSumInt64(ord uint64, key string, value []byte) {
	var data []byte
	val, found := b.getAt(ord, key)
	if !found {
		data = value
	} else {
		switch string(value[:4]) {
		case "sum:":
			prevPrefix := string(val[:4]) // if we had a 'set:' before, we keep it.
			prev, _ := strconv.ParseInt(string(val[4:]), 10, 64)
			next, _ := strconv.ParseInt(string(value[4:]), 10, 64)
			data = []byte(prevPrefix + strconv.FormatInt(prev+next, 10))
		case "set:":
			data = value
		default:

		}
	}
	b.set(ord, key, data)
}

func (b *baseStore) SetSumFloat64(ord uint64, key string, value []byte) {
	b.kvOps.Add(&pbssinternal.Operation{
		Type:  pbssinternal.Operation_SET_SUM_FLOAT64,
		Ord:   ord,
		Key:   key,
		Value: value,
	})
}

func (b *baseStore) setSumFloat64(ord uint64, key string, value []byte) {
	var data []byte
	val, found := b.getAt(ord, key)
	if !found {
		data = value
	} else {
		switch string(value[:4]) {
		case "sum:":
			prevPrefix := string(val[:4]) // if we had a 'set:' before, we keep it.
			prev, _ := strconv.ParseFloat(string(val[4:]), 64)
			next, _ := strconv.ParseFloat(string(value[4:]), 64)
			data = []byte(prevPrefix + strconv.FormatFloat(prev+next, 'g', 100, 64))
		case "set:":
			data = value
		default:

		}
	}
	b.set(ord, key, data)
}

func (b *baseStore) SetSumBigInt(ord uint64, key string, value []byte) {
	b.kvOps.Add(&pbssinternal.Operation{
		Type:  pbssinternal.Operation_SET_SUM_BIG_INT,
		Ord:   ord,
		Key:   key,
		Value: value,
	})
}

func (b *baseStore) setSumBigInt(ord uint64, key string, value []byte) {
	var data []byte
	val, found := b.getAt(ord, key)
	if !found {
		data = value
	} else {
		switch string(value[:4]) {
		case "sum:":
			prevPrefix := string(val[:4]) // if we had a 'set:' before, we keep it.
			prev := valueToBigInt(val[4:])
			next := valueToBigInt(value[4:])
			data = []byte(prevPrefix + big.NewInt(0).Add(prev, next).String())
		case "set:":
			data = value
		default:

		}
	}
	b.set(ord, key, data)
}

func (b *baseStore) SetSumBigDecimal(ord uint64, key string, value []byte) {
	b.kvOps.Add(&pbssinternal.Operation{
		Type:  pbssinternal.Operation_SET_SUM_BIG_DECIMAL,
		Ord:   ord,
		Key:   key,
		Value: value,
	})
}

func (b *baseStore) setSumBigDecimal(ord uint64, key string, value []byte) {
	var data []byte
	val, found := b.getAt(ord, key)
	if !found {
		data = value
	} else {
		switch string(value[:4]) {
		case "sum:":
			prevPrefix := string(val[:4]) // if we had a 'set:' before, we keep it.
			prev := mustDecimalFromBytes(val[4:])
			next := mustDecimalFromBytes(value[4:])
			data = []byte(prevPrefix + prev.Add(next).String())
		case "set:":
			data = value
		default:

		}
	}
	b.set(ord, key, data)
}

func mustDecimalFromBytes(value []byte) decimal.Decimal {
	v, err := decimal.NewFromString(string(value))
	if err != nil {
		panic(err)
	}
	return v
}
