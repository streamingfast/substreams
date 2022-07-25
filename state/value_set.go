package state

import (
	"fmt"
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
	// FIXME(abourget): these should return an error up the stack instead, would bubble up
	// in the wasm/module.go and fail the query, with proper error propagation.
	if strings.HasPrefix(key, "__!__") {
		panic("key prefix __!__ is reserved for internal system use.")
	}
	if len(value) > 10*1024*1024 {
		panic(fmt.Sprintf("key %q attempted to write %d bytes, capped at 10MiB", key, len(value)))
	}
	s.bumpOrdinal(ord)

	val, found := s.GetLast(key)

	var delta *pbsubstreams.StoreDelta
	if found {
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
	s.ApplyDelta(delta)
	s.Deltas = append(s.Deltas, delta)
}
