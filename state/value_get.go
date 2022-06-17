package state

import (
	"fmt"

	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
)

func (s *Store) GetFirst(key string) ([]byte, bool) {
	for _, delta := range s.Deltas {
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
	return s.GetLast(key)
}

func (s *Store) GetLast(key string) ([]byte, bool) {
	val, found := s.KV[key]
	return val, found
}

// GetAt returns the key for the state that includes the processing of `ord`.
func (s *Store) GetAt(ord uint64, key string) (out []byte, found bool) {
	out, found = s.GetLast(key)

	for i := len(s.Deltas) - 1; i >= 0; i-- {
		delta := s.Deltas[i]
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
