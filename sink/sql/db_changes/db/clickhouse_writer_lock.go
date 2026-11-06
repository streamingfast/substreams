package db

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2"
	"go.uber.org/zap"
)

// ClickHouse writer lock
//
// The lock is a lease: a row in a KeeperMap table holding its holder and expiry, in the server's
// time. keeper_map_strict_mode makes creating the row fail if it exists, and updating it fail if
// it changed since read, so two sinks cannot both take it. The holder renews it every poll
// interval; a standby takes over a lease left to expire.
//
// Writes cannot be made conditional on the lease, so the holder checks it, with a margin, before
// each write. To cover writes already sent, a standby taking over waits one more lease period.
//
// KeeperMap needs ClickHouse Keeper (or ZooKeeper) and keeper_map_path_prefix in the server
// configuration.

const clickhouseWriterLockTable = "substreams_writer_lock"

// clickhouseMinLeaseTTL is the shortest lease; a var for tests.
var clickhouseMinLeaseTTL = 15 * time.Second

// ErrWriterLockLost is returned when the sink's lease was taken over or expired.
var ErrWriterLockLost = errors.New("this sink no longer holds the writer lock")

type clickhouseLease struct {
	holder string
	ttl    time.Duration
}

func (d ClickhouseDialect) writerLockIdentifier() string {
	return EscapeIdentifier(d.schemaName) + "." + EscapeIdentifier(clickhouseWriterLockTable)
}

func strictKeeperMap(ctx context.Context) context.Context {
	return clickhouse.Context(ctx, clickhouse.WithSettings(clickhouse.Settings{"keeper_map_strict_mode": 1, "async_insert": 0}))
}

const serverNowMillis = "toUnixTimestamp64Milli(now64(3))"

func newLeaseHolder() string {
	host, _ := os.Hostname()
	random := make([]byte, 6)
	_, _ = rand.Read(random)
	return fmt.Sprintf("%s/%d/%s", host, os.Getpid(), hex.EncodeToString(random))
}

func (l *Loader) clickhouseWriterLockKey() string {
	return strconv.FormatInt(l.WriterLockKey(), 10)
}

// acquireClickhouseWriterLock is AcquireWriterLock in ClickHouse stateful mode.
func (l *Loader) acquireClickhouseWriterLock(ctx context.Context, d *ClickhouseDialect, pollInterval time.Duration, stop <-chan struct{}) error {
	clusterClause := ""
	if d.cluster != "" {
		clusterClause = "ON CLUSTER " + EscapeIdentifier(d.cluster)
	}
	create := fmt.Sprintf(`
		CREATE TABLE IF NOT EXISTS %s %s
		(
			lock_key   String,
			holder     String,
			expires_at UInt64,
			fence      UInt64
		) ENGINE = KeeperMap('/substreams_sink/%s/writer_lock') PRIMARY KEY lock_key`,
		d.writerLockIdentifier(), clusterClause, d.schemaName)
	if _, err := l.ExecContext(ctx, create); err != nil {
		return fmt.Errorf("creating the writer lock table (KeeperMap needs ClickHouse Keeper and keeper_map_path_prefix in the server configuration): %w", err)
	}

	lease := &clickhouseLease{holder: newLeaseHolder(), ttl: max(3*pollInterval, clickhouseMinLeaseTTL)}
	key := l.clickhouseWriterLockKey()
	logger := l.logger.With(zap.String("lock_key", key), zap.String("holder", lease.holder))

	var lastStandbyLog time.Time
	for {
		taken, tookOver, current, err := l.tryTakeClickhouseLease(ctx, d, key, lease)
		switch {
		case err != nil:
			logger.Warn("unable to take the writer lock, retrying", zap.Error(err))
		case taken:
			l.clickhouseLease = lease
			if tookOver {
				// The previous holder may have writes in flight.
				logger.Info("writer lock taken over from an expired lease, waiting one lease period before writing", zap.String("previous_holder", current), zap.Duration("lease", lease.ttl))
				if err := l.holdClickhouseLeaseFor(ctx, d, lease.ttl, pollInterval, stop); err != nil {
					l.clickhouseLease = nil
					return err
				}
				if l.clickhouseLease == nil {
					return nil // stopped meanwhile
				}
			}
			logger.Info("writer lock acquired, this sink is the active writer", zap.Duration("lease", lease.ttl))
			return nil
		case time.Since(lastStandbyLog) >= time.Minute:
			logger.Info("standby: another sink holds the writer lock, waiting to take over", zap.String("current_holder", current))
			lastStandbyLog = time.Now()
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-stop:
			return nil
		case <-time.After(pollInterval):
		}
	}
}

// tryTakeClickhouseLease creates the lease, or takes it over once expired, and returns the holder
// it found.
func (l *Loader) tryTakeClickhouseLease(ctx context.Context, d *ClickhouseDialect, key string, lease *clickhouseLease) (taken, tookOver bool, previous string, err error) {
	ctx = strictKeeperMap(ctx)
	ttl := lease.ttl.Milliseconds()

	_, err = l.ExecContext(ctx, fmt.Sprintf("INSERT INTO %s (lock_key, holder, expires_at, fence) SELECT ?, ?, %s + ?, 1", d.writerLockIdentifier(), serverNowMillis),
		key, lease.holder, ttl)
	if err == nil {
		return true, false, "", nil
	}

	holder, remaining, err := l.readClickhouseLease(ctx, d, key)
	if errors.Is(err, sql.ErrNoRows) {
		return false, false, "", nil // released meanwhile, the next try creates it
	}
	if err != nil {
		return false, false, "", err
	}
	if remaining >= 0 {
		return false, false, holder, nil
	}

	// Strict mode fails the update if another sink changed the lease since it was read.
	_, err = l.ExecContext(ctx, fmt.Sprintf(
		"ALTER TABLE %s UPDATE holder = ?, expires_at = %s + ?, fence = fence + 1 WHERE lock_key = ? AND holder = ? AND expires_at < %s",
		d.writerLockIdentifier(), serverNowMillis, serverNowMillis), lease.holder, ttl, key, holder)
	if err != nil {
		return false, false, holder, fmt.Errorf("taking over the expired lease of %s: %w", holder, err)
	}
	current, _, err := l.readClickhouseLease(ctx, d, key)
	if err != nil {
		return false, false, "", err
	}
	return current == lease.holder, true, holder, nil
}

// readClickhouseLease returns the lease's holder and how long it remains valid (negative once
// expired).
func (l *Loader) readClickhouseLease(ctx context.Context, d *ClickhouseDialect, key string) (holder string, remaining time.Duration, err error) {
	var remainingMillis int64
	err = l.QueryRowContext(ctx, fmt.Sprintf("SELECT holder, toInt64(expires_at) - toInt64(%s) FROM %s WHERE lock_key = ?", serverNowMillis, d.writerLockIdentifier()), key).
		Scan(&holder, &remainingMillis)
	return holder, time.Duration(remainingMillis) * time.Millisecond, err
}

// holdClickhouseLeaseFor keeps renewing the lease for duration.
func (l *Loader) holdClickhouseLeaseFor(ctx context.Context, d *ClickhouseDialect, duration, pollInterval time.Duration, stop <-chan struct{}) error {
	deadline := time.Now().Add(duration)
	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-stop:
			l.releaseClickhouseLease(d)
			return nil
		case <-time.After(min(pollInterval, time.Until(deadline))):
		}
		if err := l.renewClickhouseLease(ctx, d); err != nil {
			return err
		}
	}
	return nil
}

// renewClickhouseLease extends the lease, failing with ErrWriterLockLost if it was lost.
func (l *Loader) renewClickhouseLease(ctx context.Context, d *ClickhouseDialect) error {
	lease := l.clickhouseLease
	ctx = strictKeeperMap(ctx)
	key := l.clickhouseWriterLockKey()
	// Only renewed while not expired: once expired, a standby may take it over.
	if _, err := l.ExecContext(ctx, fmt.Sprintf("ALTER TABLE %s UPDATE expires_at = %s + ? WHERE lock_key = ? AND holder = ? AND expires_at >= %s",
		d.writerLockIdentifier(), serverNowMillis, serverNowMillis), lease.ttl.Milliseconds(), key, lease.holder); err != nil {
		return err
	}
	return l.checkClickhouseLease(ctx, d, 0)
}

// checkClickhouseLease fails with ErrWriterLockLost unless the lease is this sink's and stays valid
// for at least margin.
func (l *Loader) checkClickhouseLease(ctx context.Context, d *ClickhouseDialect, margin time.Duration) error {
	holder, remaining, err := l.readClickhouseLease(ctx, d, l.clickhouseWriterLockKey())
	if errors.Is(err, sql.ErrNoRows) || (err == nil && (holder != l.clickhouseLease.holder || remaining < margin)) {
		return ErrWriterLockLost
	}
	return err
}

// checkWriterLease makes sure this sink still holds the lease, if it took one, with a margin.
func (l *Loader) checkWriterLease(ctx context.Context, d ClickhouseDialect) error {
	if l.clickhouseLease == nil {
		return nil
	}
	return l.checkClickhouseLease(ctx, &d, l.clickhouseLease.ttl/3)
}

// beforeCursorWrite checks the lease and tags the cursor write (see sinkWrite).
func (d ClickhouseDialect) beforeCursorWrite(ctx context.Context, l *Loader) (context.Context, error) {
	if err := l.checkWriterLease(ctx, d); err != nil {
		return ctx, err
	}
	if !d.isStateful() {
		return ctx, nil
	}
	return d.sinkWrite(ctx), nil
}

func (l *Loader) releaseClickhouseLease(d *ClickhouseDialect) {
	ctx, cancel := context.WithTimeout(strictKeeperMap(context.Background()), 5*time.Second)
	defer cancel()
	if _, err := l.ExecContext(ctx, fmt.Sprintf("DELETE FROM %s WHERE lock_key = ? AND holder = ?", d.writerLockIdentifier()),
		l.clickhouseWriterLockKey(), l.clickhouseLease.holder); err != nil {
		l.logger.Warn("unable to release the writer lock, it expires on its own", zap.Error(err))
	}
	l.clickhouseLease = nil
}
