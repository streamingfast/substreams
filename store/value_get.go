package store

import (
	"fmt"

	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
)

func (s *BaseStore) GetFirst(key string) ([]byte, bool) {
	for _, delta := range s.deltas {
		if delta.Key == key {
			switch delta.Operation {
			case pbsubstreams.StoreDelta_DELETE, pbsubstreams.StoreDelta_UPDATE:
				return delta.OldValue, true
			case pbsubstreams.StoreDelta_CREATE:
				return nil, false
			default:
				// WARN: is that legit? what if some upstream stream is broken? can we trust all those streams?
				panic(fmt.Sprintf("invalid value %q for pbsubstreams.StoreDelta::Op for key %q", delta.Operation.String(), delta.Key))
			}
		}
	}

	val, found := s.kv[key]
	return val, found
}

func (s *BaseStore) GetLast(key string) ([]byte, bool) {
	for i := len(s.deltas) - 1; i >= 0; i-- {
		delta := s.deltas[i]
		if delta.Key == key {
			switch delta.Operation {
			case pbsubstreams.StoreDelta_DELETE:
				return nil, false
			case pbsubstreams.StoreDelta_CREATE, pbsubstreams.StoreDelta_UPDATE:
				return delta.NewValue, true
			default:
				panic(fmt.Sprintf("invalid value %q for pbsubstreams.StoreDelta::Op for key %q", delta.Operation.String(), delta.Key))
			}
		}
	}

	val, found := s.kv[key]
	return val, found
}

// GetAt returns the key for the state that includes the processing of `ord`.
func (s *BaseStore) GetAt(ord uint64, key string) (out []byte, found bool) {
	out, found = s.GetLast(key)

	for i := len(s.deltas) - 1; i >= 0; i-- {
		delta := s.deltas[i]
		if delta.Ordinal <= ord {
			break
		}
		if delta.Key == key {
			switch delta.Operation {
			case pbsubstreams.StoreDelta_DELETE, pbsubstreams.StoreDelta_UPDATE:
				out = delta.OldValue
				found = true
			case pbsubstreams.StoreDelta_CREATE:
				out = nil
				found = false
			default:
				// WARN: is that legit? what if some upstream stream is broken? can we trust all those streams?
				panic(fmt.Sprintf("invalid value %q for pbsubstreams.StateDelta::Op for key %q", delta.Operation, delta.Key))
			}
		}
	}
	return
}
