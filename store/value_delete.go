package store

import (
	"strings"

	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
)

//func (s *KVStore) Del(ord uint64, key string) {
//	s.bumpOrdinal(ord)
//
//	val, found := s.GetLast(key)
//	if found {
//		delta := &pbsubstreams.StoreDelta{
//			Operation: pbsubstreams.StoreDelta_DELETE,
//			Ordinal:   ord,
//			Key:       key,
//			OldValue:  val,
//			NewValue:  nil,
//		}
//		s.ApplyDelta(delta)
//		s.deltas = append(s.deltas, delta)
//	}
//}

func (s *KVStore) DeletePrefix(ord uint64, prefix string) {
	s.bumpOrdinal(ord)

	for key, val := range s.kv {
		if !strings.HasPrefix(key, prefix) {
			continue
		}
		delta := &pbsubstreams.StoreDelta{
			Operation: pbsubstreams.StoreDelta_DELETE,
			Ordinal:   ord,
			Key:       key,
			OldValue:  val,
			NewValue:  nil,
		}
		s.ApplyDelta(delta)
		s.deltas = append(s.deltas, delta)
	}
}
