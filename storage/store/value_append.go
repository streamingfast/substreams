package store

import (
	"fmt"

	pbssinternal "github.com/streamingfast/substreams/pb/sf/substreams/intern/v2"
)

func (b *baseStore) Append(ord uint64, key string, value []byte) {
	b.pendingOps.Add(&pbssinternal.Operation{
		Type:  pbssinternal.Operation_APPEND,
		Ord:   ord,
		Key:   key,
		Value: cloneBytes(value),
	})
}

func (b *baseStore) append(ord uint64, key string, value []byte) error {
	var newVal []byte
	oldVal, found := b.GetAt(ord, key)
	if !found {
		newVal = make([]byte, len(value))
		copy(newVal[0:], value)
	} else {
		newLen := uint64(len(oldVal) + len(value))
		if b.appendLimit > 0 && newLen >= b.appendLimit {
			return fmt.Errorf("append would exceed limit of %d bytes", b.appendLimit)
		}

		newVal = make([]byte, len(oldVal)+len(value))
		copy(newVal[0:], oldVal)
		copy(newVal[len(oldVal):], value)
	}
	b.set(ord, key, newVal)

	return nil
}
