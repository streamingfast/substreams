package store

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestMmapGet_NilReceiverPanicMessage pins the fix for the broken nil-panic
// message in Get: the pre-fix code did `panic(fmt.Sprintf(...b.storeName))`
// inside the `b == nil` branch, which dereferences the nil receiver and yields
// a generic "invalid memory address" runtime panic instead of the intended
// diagnostic. The two error states (nil receiver vs closed/nil db) must each
// produce their own message without dereferencing a nil receiver.
func TestMmapGet_NilReceiverPanicMessage(t *testing.T) {
	var b *mmapKVImpl // nil receiver

	defer func() {
		r := recover()
		require.NotNil(t, r, "Get on nil receiver must panic")
		msg, ok := r.(string)
		require.True(t, ok, "panic value must be our string message, not a runtime error, got %T: %v", r, r)
		require.True(t, strings.Contains(msg, "nil receiver"),
			"panic must carry the intended nil-receiver message, got %q", msg)
	}()

	b.Get("anything")
}

// TestMmapGet_ClosedDBPanicMessage covers the sibling branch: a non-nil
// receiver whose db is nil (closed/uninitialized) must report the store name.
func TestMmapGet_ClosedDBPanicMessage(t *testing.T) {
	b := &mmapKVImpl{storeName: "my_store"} // non-nil receiver, nil db

	defer func() {
		r := recover()
		require.NotNil(t, r, "Get on closed db must panic")
		msg, ok := r.(string)
		require.True(t, ok, "panic value must be our string message, got %T: %v", r, r)
		require.True(t, strings.Contains(msg, "my_store"),
			"panic must include the store name, got %q", msg)
	}()

	b.Get("anything")
}
