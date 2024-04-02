package store

import (
	"fmt"

	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
)

func (b *baseStore) GetFirst(key string) ([]byte, bool) {
	for _, delta := range b.deltas {
		if delta.Key != key {
			continue
		}

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

	val, found := b.kv[key]
	return val, found
}

func (b *baseStore) HasFirst(key string) bool {
	for _, delta := range b.deltas {
		if delta.Key != key {
			continue
		}

		switch delta.Operation {
		case pbsubstreams.StoreDelta_DELETE, pbsubstreams.StoreDelta_UPDATE:
			return true
		case pbsubstreams.StoreDelta_CREATE:
			return false
		default:
			// WARN: is that legit? what if some upstream stream is broken? can we trust all those streams?
			panic(fmt.Sprintf("invalid value %q for pbsubstreams.StoreDelta::Op for key %q", delta.Operation.String(), delta.Key))
		}

	}

	_, found := b.kv[key]
	return found
}

func (b *baseStore) GetLast(key string) ([]byte, bool) {
	for i := len(b.deltas) - 1; i >= 0; i-- {
		delta := b.deltas[i]
		if delta.Key != key {
			continue
		}

		switch delta.Operation {
		case pbsubstreams.StoreDelta_DELETE:
			return nil, false
		case pbsubstreams.StoreDelta_CREATE, pbsubstreams.StoreDelta_UPDATE:
			return delta.NewValue, true
		default:
			panic(fmt.Sprintf("invalid value %q for pbsubstreams.StoreDelta::Op for key %q", delta.Operation.String(), delta.Key))
		}
	}

	val, found := b.kv[key]
	return val, found
}

func (b *baseStore) HasLast(key string) bool {
	for i := len(b.deltas) - 1; i >= 0; i-- {
		delta := b.deltas[i]
		if delta.Key != key {
			continue
		}

		switch delta.Operation {
		case pbsubstreams.StoreDelta_DELETE:
			return false
		case pbsubstreams.StoreDelta_CREATE, pbsubstreams.StoreDelta_UPDATE:
			return true
		default:
			panic(fmt.Sprintf("invalid value %q for pbsubstreams.StoreDelta::Op for key %q", delta.Operation.String(), delta.Key))
		}
	}

	_, found := b.kv[key]
	return found
}

// GetAt returns the key for the state that includes the processing of `ord`.
func (b *baseStore) GetAt(ord uint64, key string) (out []byte, found bool) {
	out, found = b.GetLast(key)

	for i := len(b.deltas) - 1; i >= 0; i-- {
		delta := b.deltas[i]
		if delta.Ordinal <= ord {
			break
		}
		if delta.Key != key {
			continue
		}

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
	return
}

// HasAt returns true if the key exists for the state that includes the processing of `ord`.
func (b *baseStore) HasAt(ord uint64, key string) bool {
	_, found := b.GetFirst(key)

	for i := len(b.deltas) - 1; i >= 0; i-- {
		delta := b.deltas[i]
		if delta.Ordinal <= ord {
			break
		}

		if delta.Key != key {
			continue
		}

		switch delta.Operation {
		case pbsubstreams.StoreDelta_DELETE, pbsubstreams.StoreDelta_UPDATE:
			found = true
		case pbsubstreams.StoreDelta_CREATE:
			found = false
		default:
			// WARN: is that legit? what if some upstream stream is broken? can we trust all those streams?
			panic(fmt.Sprintf("invalid value %q for pbsubstreams.StateDelta::Op for key %q", delta.Operation, delta.Key))
		}
	}

	return found
}
