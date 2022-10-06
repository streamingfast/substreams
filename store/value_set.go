package store

import (
	"fmt"
	"strings"

	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
)

func (s *KVStore) SetBytesIfNotExists(ord uint64, key string, value []byte) {
	s.setIfNotExists(ord, key, value)
}

func (s *KVStore) SetIfNotExists(ord uint64, key string, value string) {
	s.setIfNotExists(ord, key, []byte(value))
}

func (s *KVStore) SetBytes(ord uint64, key string, value []byte) {
	s.set(ord, key, value)
}

func (s *KVStore) Set(ord uint64, key string, value string) {
	s.set(ord, key, []byte(value))
}

func (s *KVStore) set(ord uint64, key string, value []byte) {
	// FIXME(abourget): these should return an error up the stack instead, would bubble up
	// in the wasm/module.go and fail the query, with proper error propagation.
	if strings.HasPrefix(key, "__!__") {
		panic("key prefix __!__ is reserved for internal system use.")
	}
	if len(value) > 10*1024*1024 {
		panic(fmt.Sprintf("key %q attempted to write %d bytes, capped at 10MiB", key, len(value)))
	}

	if len(key) == 0 {
		panic(fmt.Sprintf("invalid key"))
	}

	s.bumpOrdinal(ord)

	val, found := s.GetLast(key)
	cpValue := make([]byte, len(value))
	copy(cpValue, value)

	var delta *pbsubstreams.StoreDelta
	if found {
		delta = &pbsubstreams.StoreDelta{
			Operation: pbsubstreams.StoreDelta_UPDATE,
			Ordinal:   ord,
			Key:       key,
			OldValue:  val,
			NewValue:  cpValue,
		}
	} else {
		delta = &pbsubstreams.StoreDelta{
			Operation: pbsubstreams.StoreDelta_CREATE,
			Ordinal:   ord,
			Key:       key,
			OldValue:  nil,
			NewValue:  cpValue,
		}
	}

	s.ApplyDelta(delta)
	s.deltas = append(s.deltas, delta)
}

func (s *KVStore) setIfNotExists(ord uint64, key string, value []byte) {
	_, found := s.GetLast(key)
	if found {
		return
	}

	s.bumpOrdinal(ord)

	cpValue := make([]byte, len(value))
	copy(cpValue, value)

	delta := &pbsubstreams.StoreDelta{
		Operation: pbsubstreams.StoreDelta_CREATE,
		Ordinal:   ord,
		Key:       key,
		OldValue:  nil,
		NewValue:  cpValue,
	}

	s.ApplyDelta(delta)
	s.deltas = append(s.deltas, delta)
}
