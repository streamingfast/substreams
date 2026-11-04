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

// The writer lock's session must survive long stretches without flushes: an idle timeout ending it
// would hand the lock to a standby, and the sinks would take turns failing over.
func TestWriterLockKeepalive_Postgres(t *testing.T) {
	ctx := context.Background()
	dsn := pgTestSchema(t)
	poll := 50 * time.Millisecond

	newWriter := func(t *testing.T) *Loader {
		l := pgTestLoader(t, dsn, true)
		require.NoError(t, l.AcquireWriterLock(ctx, poll, nil))
		_, err := l.writerConn.ExecContext(ctx, "SET idle_session_timeout = '300ms'")
		require.NoError(t, err)
		return l
	}
	lockTaken := func(t *testing.T, within time.Duration) bool {
		other := pgTestLoader(t, dsn, true)
		ctx, cancel := context.WithTimeout(ctx, within)
		defer cancel()
		err := other.AcquireWriterLock(ctx, poll, nil)
		if err == nil {
			other.ReleaseWriterLock()
		}
		return err == nil
	}

	t.Run("without the keepalive, an idle session loses the lock", func(t *testing.T) {
		newWriter(t)
		time.Sleep(time.Second)
		require.True(t, lockTaken(t, 2*time.Second), "the idle timeout should have ended the session")
	})

	t.Run("the keepalive keeps an idle session and its lock", func(t *testing.T) {
		a := newWriter(t)
		require.NoError(t, a.InsertCursor(ctx, testModuleHash, sink.NewBlankCursor()))
		a.SetExpectedCursor(sink.NewBlankCursor().String())
		stop := make(chan struct{})
		done := make(chan error, 1)
		go func() {
			ticker := time.NewTicker(100 * time.Millisecond)
			defer ticker.Stop()
			for {
				select {
				case <-stop:
					done <- nil
					return
				case <-ticker.C:
					if err := a.KeepWriterSessionAlive(ctx, time.Second); err != nil {
						done <- err
						return
					}
				}
			}
		}()
		require.False(t, lockTaken(t, 1200*time.Millisecond), "another sink took the lock")
		close(stop)
		require.NoError(t, <-done)

		add(t, a, "x", "1")
		_, err := a.Flush(ctx, testModuleHash, pgCursor(10, 10), 10)
		require.NoError(t, err, "the writer can still write")
		a.ReleaseWriterLock()
	})

	t.Run("the keepalive stays out of an open transaction", func(t *testing.T) {
		a := newWriter(t)
		_, err := a.writerConn.ExecContext(ctx, "SET idle_session_timeout = 0")
		require.NoError(t, err)
		tx, err := a.BeginTx(ctx, nil)
		require.NoError(t, err)
		_, err = tx.ExecContext(ctx, "SELECT 1/0") // leaves the transaction aborted
		require.Error(t, err)

		require.NoError(t, a.KeepWriterSessionAlive(ctx, time.Second), "skipped, not run in the aborted transaction")
		require.NoError(t, tx.Rollback())
		require.NoError(t, a.KeepWriterSessionAlive(ctx, time.Second), "runs once the transaction ended")
		require.True(t, a.WriterSessionAlive(ctx))
		a.ReleaseWriterLock()
	})

	t.Run("the keepalive stays out of a cursor read", func(t *testing.T) {
		a := newWriter(t)
		_, err := a.writerConn.ExecContext(ctx, "SET idle_session_timeout = 0")
		require.NoError(t, err)
		// Enough cursor rows that reading them takes a while.
		_, err = a.writerConn.ExecContext(ctx, fmt.Sprintf(
			"INSERT INTO %s (id, cursor, block_num, block_id) SELECT 'm' || g, '', 0, '' FROM generate_series(1, 20000) g",
			a.cursorTable.identifier,
		))
		require.NoError(t, err)

		stop := make(chan struct{})
		done := make(chan error, 1)
		go func() {
			for {
				select {
				case <-stop:
					done <- nil
					return
				default:
				}
				if err := a.KeepWriterSessionAlive(ctx, time.Second); err != nil {
					done <- err
					return
				}
			}
		}()
		for range 10 {
			_, err := a.allCursorRows(ctx)
			require.NoError(t, err)
		}
		close(stop)
		require.NoError(t, <-done, "the keepalive ran between the rows of the cursor read")
		a.ReleaseWriterLock()
	})

	t.Run("the keepalive reports a lost session", func(t *testing.T) {
		a := newWriter(t)
		var pid int
		require.NoError(t, a.writerConn.QueryRowContext(ctx, "SELECT pg_backend_pid()").Scan(&pid))
		other := pgTestLoader(t, dsn, true)
		_, err := other.DB.Exec("SELECT pg_terminate_backend($1)", pid)
		require.NoError(t, err)

		err = a.KeepWriterSessionAlive(ctx, time.Second)
		require.Error(t, err)
		require.False(t, IsCancellation(err), "a lost session is not a timeout: %v", err)
	})
}
