package state

import (
	"bytes"
	"strings"

	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
)

func (s *Store) SetBytesIfNotExists(ord uint64, key string, value []byte) {
	s.setIfNotExists(ord, key, value)
}

func (s *Store) SetIfNotExists(ord uint64, key string, value string) {
	s.setIfNotExists(ord, key, []byte(value))
}

func (s *Store) SetBytes(ord uint64, key string, value []byte) {
	s.set(ord, key, value)
}
func (s *Store) Set(ord uint64, key string, value string) {
	s.set(ord, key, []byte(value))
}

func (s *Store) set(ord uint64, key string, value []byte) {
	if strings.HasPrefix(key, "__!__") {
		panic("key prefix __!__ is reserved for internal system use.")
	}
	s.bumpOrdinal(ord)

	val, found := s.GetLast(key)

	var delta *pbsubstreams.StoreDelta
	if found {
		//Uncomment when finished debugging:
		if bytes.Compare(value, val) == 0 {
			return
		}
		delta = &pbsubstreams.StoreDelta{
			Operation: pbsubstreams.StoreDelta_UPDATE,
			Ordinal:   ord,
			Key:       key,
			OldValue:  val,
			NewValue:  value,
		}
	} else {
		delta = &pbsubstreams.StoreDelta{
			Operation: pbsubstreams.StoreDelta_CREATE,
			Ordinal:   ord,
			Key:       key,
			OldValue:  nil,
			NewValue:  value,
		}
	}
	if delta.Key == "" {
		panic("Grrr4")
	}

	s.ApplyDelta(delta)
	s.Deltas = append(s.Deltas, delta)
}

func (s *Store) setIfNotExists(ord uint64, key string, value []byte) {
	s.bumpOrdinal(ord)

	_, found := s.GetLast(key)
	if found {
		return
	}

	delta := &pbsubstreams.StoreDelta{
		Operation: pbsubstreams.StoreDelta_CREATE,
		Ordinal:   ord,
		Key:       key,
		OldValue:  nil,
		NewValue:  value,
	}
	if delta.Key == "" {
		panic("Grrr4")
	}
	s.ApplyDelta(delta)
	s.Deltas = append(s.Deltas, delta)
}
