package store

import (
	"math/big"
	"strconv"
)

func (s *KVStore) SumBigInt(ord uint64, key string, value *big.Int) {
	sum := new(big.Int)
	val, found := s.GetAt(ord, key)
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
	s.set(ord, key, []byte(sum.String()))
}

func (s *KVStore) SumInt64(ord uint64, key string, value int64) {
	var sum int64
	val, found := s.GetAt(ord, key)
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
	s.set(ord, key, []byte(strconv.FormatInt(sum, 10)))
}

func (s *KVStore) SumFloat64(ord uint64, key string, value float64) {
	var sum float64
	val, found := s.GetAt(ord, key)
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
	s.set(ord, key, []byte(strconv.FormatFloat(sum, 'g', 100, 64)))
}

func (s *KVStore) SumBigFloat(ord uint64, key string, value *big.Float) {
	sum := new(big.Float)
	val, found := s.GetAt(ord, key)
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
	s.set(ord, key, []byte(sum.Text('g', 100)))
}
