package state

import (
	"strings"

	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
)

func (s *Store) Del(ord uint64, key string) {
	s.bumpOrdinal(ord)

	val, found := s.GetLast(key)
	if found {
		delta := &pbsubstreams.StoreDelta{
			Operation: pbsubstreams.StoreDelta_DELETE,
			Ordinal:   ord,
			Key:       key,
			OldValue:  val,
			NewValue:  nil,
		}
		s.ApplyDelta(delta)
		s.Deltas = append(s.Deltas, delta)
	}
}

func (s *Store) DeletePrefix(ord uint64, prefix string) {
	s.bumpOrdinal(ord)

	for key, val := range s.KV {
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
		s.Deltas = append(s.Deltas, delta)

	}

	if s.IsPartial() {
		s.DeletedPrefixes = append(s.DeletedPrefixes, prefix)
	}
}
