package store

import (
	"sort"
	"strings"

	pbssinternal "github.com/streamingfast/substreams/pb/sf/substreams/intern/v2"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
)

func (b *baseStore) DeletePrefix(ord uint64, prefix string) {
	b.pendingOps.Add(&pbssinternal.Operation{
		Type: pbssinternal.Operation_DELETE_PREFIX,
		Ord:  ord,
		Key:  prefix,
	})
}

func (b *baseStore) deletePrefix(ord uint64, prefix string) {

	var deltas []*pbsubstreams.StoreDelta
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
		deltas = append(deltas, delta)
	}
	sort.Slice(deltas, func(i, j int) bool {
		return deltas[i].Key < deltas[j].Key
	})
	b.deltas = append(b.deltas, deltas...)
}
