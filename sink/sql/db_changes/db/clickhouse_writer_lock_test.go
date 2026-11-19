package db

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The writer lock needs ClickHouse Keeper: CH_KEEPER_DSN points at a server with it (and
// keeper_map_path_prefix), without database.
func newKeeperTestLoaders(t *testing.T) (*Loader, *Loader) {
	t.Helper()
	dsn := os.Getenv("CH_KEEPER_DSN")
	if dsn == "" {
		t.Skip(`CH_KEEPER_DSN not set, please specify CH_KEEPER_DSN (a ClickHouse server with Keeper) to run this test`)
	}
	t.Setenv("CH_DSN", dsn)
	a, database := newStatefulTestLoader(t, "")
	b, _ := newStatefulTestLoader(t, database)
	return a, b
}

func TestClickhouseWriterLock_StandbyTakesOverOnRelease(t *testing.T) {
	a, b := newKeeperTestLoaders(t)
	ctx := context.Background()

	require.NoError(t, a.AcquireWriterLock(ctx, 100*time.Millisecond, nil))
	require.True(t, a.HoldsWriterLock())

	stop := make(chan struct{})
	go func() { time.Sleep(500 * time.Millisecond); close(stop) }()
	require.NoError(t, b.AcquireWriterLock(ctx, 100*time.Millisecond, stop))
	assert.False(t, b.HoldsWriterLock(), "a standby must wait while the lock is held")

	a.ReleaseWriterLock()
	require.NoError(t, b.AcquireWriterLock(ctx, 100*time.Millisecond, nil))
	assert.True(t, b.HoldsWriterLock())
	b.ReleaseWriterLock()
}

func TestClickhouseWriterLock_ExpiredLeaseTakenOverAndOldHolderStops(t *testing.T) {
	previous := clickhouseMinLeaseTTL
	clickhouseMinLeaseTTL = time.Second
	t.Cleanup(func() { clickhouseMinLeaseTTL = previous })

	a, b := newKeeperTestLoaders(t)
	ctx := context.Background()

	require.NoError(t, a.AcquireWriterLock(ctx, 100*time.Millisecond, nil))
	require.NoError(t, a.KeepWriterSessionAlive(ctx, time.Second), "renewing its own lease")
	require.NoError(t, a.Upsert("balances", map[string]string{"id": "x"}, map[string]FieldData{"amount": {Value: "1"}}, nil))
	require.NoError(t, flushStatefulTest(t, a, 2))

	// a stops renewing (stalled, partitioned): its lease expires and b takes over.
	time.Sleep(1500 * time.Millisecond)
	require.NoError(t, b.AcquireWriterLock(ctx, 100*time.Millisecond, nil))
	require.True(t, b.HoldsWriterLock())

	// a comes back: it cannot write anymore.
	require.NoError(t, a.Upsert("balances", map[string]string{"id": "x"}, map[string]FieldData{"amount": {Value: "1", UpdateOp: UpdateOpAdd}}, nil))
	_, err := a.Flush(ctx, testModuleHash, pgCursor(3, 3), 3)
	require.ErrorIs(t, err, ErrWriterLockLost)
	assert.False(t, a.WriterSessionAlive(ctx))
	assert.ErrorIs(t, a.KeepWriterSessionAlive(ctx, time.Second), ErrWriterLockLost)
	assert.Equal(t, map[string]string{"x": "1/NULL"}, balances(t, b))
}

func TestClickhouseStateful_CursorConflict(t *testing.T) {
	a, database := newStatefulTestLoader(t, "")
	b, _ := newStatefulTestLoader(t, database)

	require.NoError(t, a.Upsert("balances", map[string]string{"id": "x"}, map[string]FieldData{"amount": {Value: "1"}}, nil))
	require.NoError(t, flushStatefulTest(t, a, 2))

	// b read the cursor before a wrote: a second writer.
	require.NoError(t, b.Upsert("balances", map[string]string{"id": "x"}, map[string]FieldData{"amount": {Value: "1", UpdateOp: UpdateOpAdd}}, nil))
	_, err := b.Flush(context.Background(), testModuleHash, pgCursor(2, 2), 2)
	require.ErrorIs(t, err, ErrCursorConflict)
	assert.Equal(t, map[string]string{"x": "1/NULL"}, balances(t, a))
}

func TestClickhouseWriterLock_OneOfManyRacingStandbys(t *testing.T) {
	previous := clickhouseMinLeaseTTL
	clickhouseMinLeaseTTL = time.Second
	t.Cleanup(func() { clickhouseMinLeaseTTL = previous })

	first, database := func() (*Loader, string) {
		if os.Getenv("CH_KEEPER_DSN") == "" {
			t.Skip("CH_KEEPER_DSN not set")
		}
		t.Setenv("CH_DSN", os.Getenv("CH_KEEPER_DSN"))
		return newStatefulTestLoader(t, "")
	}()
	ctx := context.Background()
	require.NoError(t, first.AcquireWriterLock(ctx, 50*time.Millisecond, nil))
	time.Sleep(1200 * time.Millisecond) // expired, never renewed

	standbys := make([]*Loader, 6)
	for i := range standbys {
		standbys[i], _ = newStatefulTestLoader(t, database)
	}
	stop := make(chan struct{})
	done := make(chan struct{}, len(standbys))
	for _, s := range standbys {
		go func() {
			_ = s.AcquireWriterLock(ctx, 20*time.Millisecond, stop)
			done <- struct{}{}
		}()
	}
	time.Sleep(1600 * time.Millisecond) // past the winner's wait of one lease period
	close(stop)
	for range standbys {
		<-done
	}

	holders := 0
	for _, s := range standbys {
		if s.HoldsWriterLock() && s.WriterSessionAlive(ctx) {
			holders++
		}
	}
	assert.Equal(t, 1, holders)
}
