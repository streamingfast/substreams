# Standby Sinks

This applies to the PostgreSQL sink in Database Changes mode (`substreams sink postgres` with a module
outputting `DatabaseChanges`).

## One writer per cursor

The sink commits each batch of rows together with its cursor, so it can always restart from the
cursor stored in the database. Two sinks writing the same module's data at once would corrupt it:
both would apply the same blocks, and the slower one would move the cursor back.

So once the sink has read or written its cursor, each flush (or revert) transaction starts by
locking the sink's cursor row and checking it still holds the cursor this sink last read or wrote.
If not, the transaction is rolled back before writing anything, and the sink stops with:

```
the stored cursor is not the one this sink last read or wrote
```

Restart the sink: it resumes from the cursor stored in the database. This happens when:

- another sink of the same module writes to the same database;
- the database lost commits, for example after a restore or a failover with asynchronous replication;
- a commit the sink saw fail actually went through (its reply was lost). Retrying it would apply
  the same blocks twice, which matters for `ADD` fields.

Editing the sink's cursor while it runs also stops it, and it then resumes from the edited cursor.
Other rows of the cursors table are not checked, and the cursor update still comes last in the
transaction, as before, so triggers on the cursors table see the batch's rows.

Sinks of *different* modules writing to the same tables (for example a new module version started
with `--on-module-hash-mismatch=warn` while the old one still runs) each update their own cursor row,
so this check does not keep them apart: use `--standby-lock`, whose lock covers the whole cursors
table. Switching back and forth between module versions on the same tables is not supported either
way: a module resuming from an older cursor re-applies blocks the other one already wrote.

## Running standbys: `--standby-lock`

With `--standby-lock`, a sink takes a writer lock before streaming: a PostgreSQL session advisory lock
keyed on the cursors table. Run the same command on several machines against the same database:

- the first sink to get the lock streams and writes;
- the others wait as standbys, without opening a Substreams stream, checking every
  `--standby-lock-poll-interval` (default `2s`);
- when the lock is released, the first standby to see it takes over, reading the cursor stored in
  the database and streaming from there.

The lock is released when the active sink stops, or when its database session ends. The active sink
reads the cursor and writes through the session holding the lock, so a sink that loses its session
loses the lock and its ability to write at the same time; it then stops, and should be restarted by
a supervisor (systemd, Docker restart policy...). It comes back as a standby.

Things to set up:

- **Dead clients:** if the machine running the active sink dies, the server only ends its session
  once TCP keepalives give up. Set the server's `tcp_keepalives_idle`, `tcp_keepalives_interval` and
  `tcp_keepalives_count` (e.g. `10`, `5`, `3`): with a direct connection, a standby then takes over
  within about 25 seconds. Through a TCP proxy such as HAProxy, the server's peer is the proxy, so
  the session lasts until the proxy closes the connection of the vanished client: enable client-side
  TCP keepalives on the proxy and set its client timeout accordingly. `client_connection_check_interval`
  also ends the session of a client that disconnected during a long query.
- **Connection:** the lock needs a session, so connect directly, through a TCP proxy, or through a
  session-mode pooler (PgBouncer's default mode). Transaction or statement pooling does not work.
- **Database failover:** advisory locks are not replicated. After a failover every sink loses its
  session; the first to reach the new primary takes the lock and resumes from the cursor stored
  there, replaying any blocks the failover lost. This relies on the old primary being fenced (for
  example Patroni with a watchdog): if two databases accept writes, each grants its own lock and its
  own cursor check passes.
- **Stalls:** a sink that is stuck but keeps its session keeps the lock. Use `--stall-timeout` (for
  example `2m`) to stop the sink when the stream delivers no block or undo for that long once live,
  which also happens while a database call hangs. The stop cancels the call in progress; the CLI
  exits at most 30 seconds later even if the call does not return.
- **Network cuts:** a sink cut off from the database cannot write. Cancelling a query needs the
  server to be reachable, so a fully cut-off sink stops when `--stall-timeout` fires and the CLI's
  30-second shutdown limit passes, or earlier if its connection errors out. A standby can take over
  as soon as the server has ended the cut-off session.

The metric `substreams_sink_postgres_writer_lock_held` is `1` on the active sink and `0` on
standbys, which serve the `--prometheus-addr` endpoint while they wait. A standby logs the database
session holding the lock every minute.
