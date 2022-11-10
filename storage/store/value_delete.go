package store

import (
	"strings"

	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
)

//func (s *baseStore) Del(ord uint64, key string) {
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

func (b *baseStore) DeletePrefix(ord uint64, prefix string) {
	b.bumpOrdinal(ord)

	for key, val := range b.kv {
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
		b.ApplyDelta(delta)
		b.deltas = append(b.deltas, delta)
	}
}
