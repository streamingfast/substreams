package db

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"hash/fnv"
	"time"

	"github.com/lib/pq"
	"go.uber.org/zap"
)

// ErrWriterLockUnsupported is returned by AcquireWriterLock for a database other than PostgreSQL.
var ErrWriterLockUnsupported = errors.New("the writer lock is only supported on PostgreSQL")

// WriterLockKey is the advisory lock key of a cursors table: one writer per cursors table, whatever
// the module hash, since sinks writing different modules to one schema share its tables.
func (l *Loader) WriterLockKey() int64 {
	h := fnv.New64a()
	h.Write([]byte("substreams-sink-sql writer lock " + l.cursorTable.identifier))
	return int64(h.Sum64())
}

// AcquireWriterLock waits until this loader holds the writer lock of its cursors table, a PostgreSQL
// session advisory lock. While another sink holds it, this one is a standby: it checks every
// pollInterval and takes over once the lock is released, which happens when that sink stops or its
// database session ends (the server closes a dead client's session once TCP keepalives give up).
//
// The lock is held on a dedicated connection, and cursor reads and all writes then go through it: a
// sink that loses its session loses the lock and its ability to write at the same time. The
// conditional cursor update (see UpdateCursor) still guards against any other writer.
//
// It returns when the lock is held, or with the context's error, or nil if stop is closed first
// (check HoldsWriterLock).
func (l *Loader) AcquireWriterLock(ctx context.Context, pollInterval time.Duration, stop <-chan struct{}) error {
	if _, ok := l.dialect.(*PostgresDialect); !ok {
		return ErrWriterLockUnsupported
	}

	key := l.WriterLockKey()
	logger := l.logger.With(zap.String("cursors_table", l.cursorTable.identifier), zap.Int64("lock_key", key))

	var conn *sql.Conn
	var lastStandbyLog time.Time
	for {
		if conn == nil {
			var err error
			if conn, err = l.DB.Conn(ctx); err != nil {
				logger.Warn("unable to connect to the database to take the writer lock, retrying", zap.Error(err))
				conn = nil
			}
		}

		if conn != nil {
			var locked bool
			if err := conn.QueryRowContext(ctx, "SELECT pg_try_advisory_lock($1)", key).Scan(&locked); err != nil {
				logger.Warn("unable to take the writer lock, retrying", zap.Error(err))
				discard(conn) // the lock may have been granted before the error
				conn = nil
			} else if locked {
				l.writerConn = conn
				logger.Info("writer lock acquired, this sink is the active writer")
				return nil
			} else if time.Since(lastStandbyLog) >= time.Minute {
				logger.Info("standby: another sink holds the writer lock, waiting to take over", l.writerLockHolder(ctx, conn, key)...)
				lastStandbyLog = time.Now()
			}
		}

		select {
		case <-ctx.Done():
			if conn != nil {
				discard(conn)
			}
			return ctx.Err()
		case <-stop:
			if conn != nil {
				discard(conn)
			}
			return nil
		case <-time.After(pollInterval):
		}
	}
}

// HoldsWriterLock reports whether AcquireWriterLock succeeded and the lock was not released since.
func (l *Loader) HoldsWriterLock() bool {
	return l.writerConn != nil
}

// WriterSessionAlive reports whether the session holding the writer lock still answers. Once it is
// gone, so is the lock, and this loader cannot write anymore.
func (l *Loader) WriterSessionAlive(ctx context.Context) bool {
	if l.writerConn == nil {
		return false
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	return l.writerConn.PingContext(ctx) == nil
}

// KeepWriterSessionAlive runs a trivial query on the session holding the writer lock, unless a
// transaction or another query's rows are open on it (the session is busy then, not idle). Called regularly, it keeps the
// session from looking idle during long stretches without flushes (large segments while catching up,
// a slow chain, sparse data), so a proxy's idle timeout or the server's idle_session_timeout doesn't
// end it, which would hand the lock to a standby and make the sinks take turns failing over.
//
// An error means the query failed: a connection error means the session, and the lock, are gone.
func (l *Loader) KeepWriterSessionAlive(ctx context.Context, timeout time.Duration) error {
	if !l.writerConnMu.TryLock() {
		return nil
	}
	defer l.writerConnMu.Unlock()
	if l.writerConn == nil {
		return nil
	}

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	_, err := l.writerConn.ExecContext(ctx, "SELECT 1")
	return err
}

// writerLockHolder describes the session holding the writer lock, for logs.
func (l *Loader) writerLockHolder(ctx context.Context, conn *sql.Conn, key int64) []zap.Field {
	// A bigint advisory key shows in pg_locks as classid (high 32 bits) and objid (low 32 bits).
	var pid int64
	var application, client string
	err := conn.QueryRowContext(ctx, `
		SELECT a.pid, COALESCE(a.application_name, ''), COALESCE(host(a.client_addr), 'local')
		FROM pg_locks k JOIN pg_stat_activity a ON a.pid = k.pid
		WHERE k.locktype = 'advisory' AND k.granted AND k.objsubid = 1 AND k.classid = $1 AND k.objid = $2
		  AND k.database = (SELECT oid FROM pg_database WHERE datname = current_database())`,
		int64(uint64(key)>>32), int64(uint32(key)),
	).Scan(&pid, &application, &client)
	if err != nil {
		return []zap.Field{zap.NamedError("holder_lookup_error", err)}
	}
	return []zap.Field{zap.Int64("holder_pid", pid), zap.String("holder_application_name", application), zap.String("holder_client", client)}
}

// ReleaseWriterLock releases the writer lock if held. Call it once no transaction is running. Closing
// the database ends the session anyway; unlocking first keeps a pooled connection from carrying the
// lock.
func (l *Loader) ReleaseWriterLock() {
	if l.writerConn == nil {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if _, err := l.writerConn.ExecContext(ctx, "SELECT pg_advisory_unlock($1)", l.WriterLockKey()); err != nil {
		l.logger.Warn("unable to release the writer lock, ending its session instead", zap.Error(err))
		discard(l.writerConn)
	} else {
		l.writerConn.Close()
	}
	l.writerConn = nil
}

// discard ends conn's session instead of returning it to the pool: a session that may hold the
// writer lock must not be reused for anything else.
func discard(conn *sql.Conn) {
	_ = conn.Raw(func(any) error { return driver.ErrBadConn })
	_ = conn.Close()
}

// IsCancellation reports whether err is how a database call reports that its context was cancelled:
// the context's error itself, the transaction rolled back on cancellation, or a PostgreSQL statement
// cancelled on request (SQLSTATE 57014). Other errors, such as ErrCursorConflict or a constraint
// violation, are real failures even if they happen while stopping.
func IsCancellation(err error) bool {
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) || errors.Is(err, sql.ErrTxDone) {
		return true
	}
	var pqErr *pq.Error
	return errors.As(err, &pqErr) && pqErr.Code == "57014"
}
