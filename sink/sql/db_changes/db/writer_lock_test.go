package db

import (
	"context"
	"database/sql"
	"fmt"
	"io"
	"testing"
	"time"

	"github.com/lib/pq"
	sink "github.com/streamingfast/substreams/sink"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWriterLock_Postgres(t *testing.T) {
	ctx := context.Background()
	dsn := pgTestSchema(t)
	poll := 50 * time.Millisecond

	a := pgTestLoader(t, dsn, true)
	require.NoError(t, a.AcquireWriterLock(ctx, poll, nil))
	require.True(t, a.HoldsWriterLock())
	require.NoError(t, a.InsertCursor(ctx, testModuleHash, sink.NewBlankCursor()))
	a.SetExpectedCursor(sink.NewBlankCursor().String())

	b := pgTestLoader(t, dsn, true)
	acquired := make(chan error, 1)
	go func() { acquired <- b.AcquireWriterLock(ctx, poll, nil) }()

	select {
	case err := <-acquired:
		t.Fatalf("standby took the lock while it was held: %v", err)
	case <-time.After(10 * poll):
	}

	t.Run("the active sink writes through the lock's session", func(t *testing.T) {
		add(t, a, "x", "1")
		_, err := a.Flush(ctx, testModuleHash, pgCursor(10, 10), 10)
		require.NoError(t, err)
	})

	t.Run("the standby takes over when the active sink's session ends", func(t *testing.T) {
		// Ending the session from the server side, as for a dead client
		var pid int
		require.NoError(t, a.writerConn.QueryRowContext(ctx, "SELECT pg_backend_pid()").Scan(&pid))
		_, err := b.DB.Exec("SELECT pg_terminate_backend($1)", pid)
		require.NoError(t, err)

		select {
		case err := <-acquired:
			require.NoError(t, err)
		case <-time.After(5 * time.Second):
			t.Fatal("standby did not take over")
		}
		require.True(t, b.HoldsWriterLock())

		// The old writer lost its session, so it cannot write anymore
		add(t, a, "x", "1")
		_, err = a.Flush(ctx, testModuleHash, pgCursor(11, 11), 11)
		require.Error(t, err)

		cursor := readCursor(t, b)
		assert.Equal(t, uint64(10), cursor.Block().Num())
		add(t, b, "x", "1")
		_, err = b.Flush(ctx, testModuleHash, pgCursor(11, 11), 11)
		require.NoError(t, err)
		assert.Equal(t, "2", pgAccValue(t, b, "x"))
	})

	t.Run("a sink of another module on the same cursors table waits too", func(t *testing.T) {
		other := pgTestLoader(t, dsn, true)
		ctx, cancel := context.WithTimeout(ctx, 10*poll)
		defer cancel()
		require.ErrorIs(t, other.AcquireWriterLock(ctx, poll, nil), context.DeadlineExceeded)
		require.False(t, other.HoldsWriterLock())
	})

	t.Run("a standby stops waiting when told to", func(t *testing.T) {
		c := pgTestLoader(t, dsn, true)
		stop := make(chan struct{})
		done := make(chan error, 1)
		go func() { done <- c.AcquireWriterLock(ctx, poll, stop) }()
		close(stop)
		select {
		case err := <-done:
			require.NoError(t, err)
			require.False(t, c.HoldsWriterLock())
		case <-time.After(5 * time.Second):
			t.Fatal("standby did not stop")
		}
	})

	t.Run("closing releases the lock", func(t *testing.T) {
		require.NoError(t, b.Close())
		d := pgTestLoader(t, dsn, true)
		ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		require.NoError(t, d.AcquireWriterLock(ctx, poll, nil))
	})
}

func TestIsCancellation(t *testing.T) {
	assert.True(t, IsCancellation(context.Canceled))
	assert.True(t, IsCancellation(fmt.Errorf("flush: %w", context.DeadlineExceeded)))
	assert.True(t, IsCancellation(fmt.Errorf("commit: %w", sql.ErrTxDone)))
	assert.True(t, IsCancellation(fmt.Errorf("dialect flush: %w", &pq.Error{Code: "57014", Message: "canceling statement due to user request"})))

	assert.False(t, IsCancellation(fmt.Errorf("flush: %w", ErrCursorConflict)))
	assert.False(t, IsCancellation(&pq.Error{Code: "23505", Message: "duplicate key value violates unique constraint"}))
	assert.False(t, IsCancellation(io.EOF))
}

// A connection whose lock state is uncertain is discarded, not returned to the pool, where its session
// would keep the lock while nobody knows it.
func TestDiscardEndsLockedSession_Postgres(t *testing.T) {
	ctx := context.Background()
	dsn := pgTestSchema(t)

	a := pgTestLoader(t, dsn, true)
	conn, err := a.DB.Conn(ctx)
	require.NoError(t, err)
	_, err = conn.ExecContext(ctx, "SELECT pg_advisory_lock($1)", a.WriterLockKey())
	require.NoError(t, err)
	discard(conn)

	b := pgTestLoader(t, dsn, true)
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	require.NoError(t, b.AcquireWriterLock(ctx, 50*time.Millisecond, nil), "the discarded session must not keep the lock")
}
