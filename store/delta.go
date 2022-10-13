package store

import (
	"fmt"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
)

func (s *KVStore) ApplyDelta(delta *pbsubstreams.StoreDelta) {
	// Keys need to have at least one character, and mustn't start with 0xFF is reserved for internal use.
	if len(delta.Key) == 0 {
		panic(fmt.Sprintf("key invalid, must be at least 1 character for module %q", s.name))
	}
	if delta.Key[0] == byte(255) {
		panic(fmt.Sprintf("key %q invalid, must be at least 1 character and not start with 0xFF", delta.Key))
	}

	switch delta.Operation {
	case pbsubstreams.StoreDelta_UPDATE, pbsubstreams.StoreDelta_CREATE:
		s.kv[delta.Key] = delta.NewValue
	case pbsubstreams.StoreDelta_DELETE:
		delete(s.kv, delta.Key)
	}
}

func (s *KVStore) ApplyDeltasReverse(deltas []*pbsubstreams.StoreDelta) {
	for i := len(deltas) - 1; i >= 0; i-- {
		delta := deltas[i]
		switch delta.Operation {
		case pbsubstreams.StoreDelta_UPDATE, pbsubstreams.StoreDelta_DELETE:
			s.kv[delta.Key] = delta.OldValue
		case pbsubstreams.StoreDelta_CREATE:
			delete(s.kv, delta.Key)
		}
	}
}

func (k *KVStore) GetDeltas() []*pbsubstreams.StoreDelta {
	return k.deltas
}

func (k *KVStore) SetDeltas(in []*pbsubstreams.StoreDelta) {
	k.deltas = in
}
