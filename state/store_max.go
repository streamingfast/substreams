package state

import (
	"fmt"
	"math/big"
	"strconv"
)

func (s *Store) SetMaxBigInt(ord uint64, key string, value *big.Int) {
	max := new(big.Int)
	val, found := s.GetAt(ord, key)
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
	s.set(ord, key, []byte(max.String()))
}

func (s *Store) SetMaxInt64(ord uint64, key string, value int64) {
	var max int64
	val, found := s.GetAt(ord, key)
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
	s.set(ord, key, []byte(fmt.Sprintf("%d", max)))
}

func (s *Store) SetMaxFloat64(ord uint64, key string, value float64) {
	var max float64
	val, found := s.GetAt(ord, key)
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
	s.set(ord, key, []byte(strconv.FormatFloat(max, 'g', 100, 64)))
}

func (s *Store) SetMaxBigFloat(ord uint64, key string, value *big.Float) {
	max := new(big.Float)
	val, found := s.GetAt(ord, key)
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
	s.set(ord, key, []byte(max.Text('g', -1)))
}
