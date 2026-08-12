# Coherent flags for the from-proto SQL sink

Status: implemented
Date: 2026-08-12
Follows: the local buffer / binary COPY work on `feature/sink-sql-local-cache`

## Why

The spool arrived as a second way of writing to the database without retiring the first
one's vocabulary. Before this plan, the flag surface said different things depending on
the driver, on whether a directory flag was set, and on where the stream happened to be:

- `--block-batch-size` used to size the database transaction. With the local buffer on —
  the default on PostgreSQL — it sizes nothing but the in-memory decode batch: transactions
  are a no-op (`postgres/database.go:366`), and the write to the database is sized by a
  hardcoded 3s target inside the buffer (`postgres/buffer/buffer.go:386`). Same flag, same
  help text, three behaviours across postgres+buffer, postgres+`--local-buffer ""`, and
  ClickHouse.
- The knobs that actually decide database load — `TargetFlushDuration` 3s,
  `SegmentMinBytes` 8MiB, `SegmentMaxBytes` 512MiB, `QueueDepth` 2 — are unreachable.
  Only `Dir` and `MaxBytes` come from flags (`cmd/substreams/sink_sql_common.go:974`).
- `QueueDepth: 2` binds long before `--local-buffer-max-size` 8GiB can: at most 3 sealed
  segments of ≤512MiB are ever in flight, so the disk high-water is ~2GiB and `awaitQuota`
  never fires at defaults. The advertised backpressure is not the one that runs; the real
  limit is the blocking channel send in `Seal`.
- `addFromProtoModeRunFlags` is registered on `setup` and on `apply-constraints`
  (`cmd/substreams/sink_postgres.go:71,75`), so two commands that never decode a block or
  write a row carry `--block-batch-size`, `--local-buffer` and `--local-buffer-max-size` —
  and `apply-constraints` carries `--apply-constraints`.
- Which write path runs is never stated by the user. It is derived from driver +
  whether `--local-buffer` is non-empty + whether the schema's foreign key graph happens
  to have a cycle (`postgres/database.go:107-115`, `:150-181`), with a `logger.Warn` on
  the downgrade.

## Principles

1. **One unit.** Everything the operator sizes is expressed against *one durable commit
   to the database*. That unit exists in all three write paths, so every knob applies
   everywhere.
2. **Three prefixes, three concerns.** `--decode-*` is CPU. `--db-write-*` is the commit
   unit. `--spool-*` is how far ahead of the database the stream may run. A flag's prefix
   says which resource it spends.
3. **Every `--decode-*`, `--db-write-*` and `--spool-*` flag has an effect in every write
   mode.** No mode-conditional flags.
4. **Backfill knobs are named as such.** At chain HEAD the sink inserts directly and
   self-bundles against the block interval; the help text says so rather than leaving the
   operator to discover it.
5. **Silence becomes a message.** A mode that cannot be honoured is an error; a mode
   chosen for the operator is logged with its reason.

## Design

### The commit unit

One commit is **one spool segment**, in every mode: its files plus the cursor, applied in
a single transaction. The mode changes only how the segment's bytes are pushed.

| write mode | how a segment is applied |
|---|---|
| `copy` | one `COPY ... FROM STDIN (FORMAT BINARY)` per table |
| `batch-insert` | multi-row INSERTs per table, ordered by foreign key |
| `row-insert` | single-row prepared INSERTs, in walk order |

So the sizer has **one dial in all three modes: segment bytes**. There is no per-mode dial
and no rows-based flag — row count falls out of the segment.
`--db-write-target-duration` runs one control loop: measure the last commit, scale the
segment target by `target/actual` (clamped 0.5–2.0), clamp to an internal floor and to
`--db-write-max-size`. This is `Buffer.resize` generalised out of the buffer package,
unchanged in shape.

The loop absorbs the cost difference between modes for free. `row-insert` is roughly an
order of magnitude more expensive per row than the multi-row path, so at the same 3s
target it simply converges on a segment about that much smaller. The operator states the
intent once — "no commit should hold my database longer than this" — and it means the same
thing whichever path is running. Exposing a rows knob would instead make them re-derive
the equivalent number per mode.

Sizing by measured duration rather than by a block count is what keeps this stable across
chains, where block payloads differ by orders of magnitude.

Two things stay internal, being implementation facts rather than operator preferences:

- **Statement chunking.** `batch-insert` splits a table's rows across statements to stay
  under PostgreSQL's 65535 bind parameters and ClickHouse's `max_query_size`.
- **The lower clamp**, a constant, not a flag. It exists only to stop a death spiral: a
  database stalled on lock contention returns a long elapsed time, the loop halves the
  segment each round, and without a floor it converges on segments whose cost is entirely
  per-segment overhead — manifest write, fsync, transaction, one statement or COPY setup
  per table. That is a correctness guard on the controller, not a policy the operator
  should have to hold an opinion about, and it is not honestly explainable as a knob:
  in `row-insert` the floor can exceed what the mode pushes in the target duration, so a
  flag named for the sizer would sometimes silently override the sizer. 8MiB, as today's
  `SegmentMinBytes`.

### Spool becomes the transport, write-mode becomes the drain

Today the on-disk spool exists only for COPY, which is what makes `--spool-*` a
copy-only concept. Invert it: the sinker always writes decoded rows to a spool segment,
and the applier goroutine drains a sealed segment using whichever write mode is selected.

```
stream → decode workers → spool segment (disk) → applier → database
         --decode-*        --spool-*             --db-write-*, --write-mode
```

Consequences:

- `--spool-dir` and `--spool-max-size` mean the same thing in every mode, on both drivers.
- `--write-mode` no longer decides *whether* rows are buffered, only *how* a sealed
  segment reaches the database. It stops being entangled with `--local-buffer`'s
  empty-string-means-off convention.
- The stream stops waiting on the database in all three modes, not just COPY.
- Blocks already downloaded survive a restart in all three modes.

### Spool format: per-mode, always pre-rendered

The spool always holds bytes that are ready to send. Rendering happens on the sinker's
side, where it is off the database's critical path and rides the decode workers; the
applier concatenates and sends.

| format | used by | contents |
|---|---|---|
| PGCOPY binary | `copy` | PostgreSQL's binary COPY wire format, one file per table |
| rendered tuples | `batch-insert`, `row-insert` | the dialect's own SQL literals, one value tuple per row, one file per table |

Two formats, three modes, both driver-aware. Pre-encoding to the target format is worth
~8% on its own for COPY (`sink/sql/db_proto/benchmarks`), and the same argument applies to
the INSERT modes, where `AccumulatorInserter` already renders values to strings before
building its statement (`postgres/accumulator_inserter.go:13-16`) — that rendering simply
moves to spool time.

ClickHouse uses the rendered-tuple format with its own dialect, so it needs no new write
mode: `--write-mode=batch-insert` is what `auto` already picks there. A future native bulk
path (ClickHouse `RowBinary` over the native protocol) would be a third format and a
fourth mode, `clickhouse-native`; out of scope here, but the applier interface should not
foreclose it.

The flags are universal; the bytes on disk are an implementation detail.

### One limit on how far ahead of the database the stream may run

`QueueDepth: 2` goes away, and no flag replaces it. A cap on the *number* of pending
segments was never the operator's concern — the disk budget is — and having two ceilings
meant the one nobody could see always won: at most 3 sealed segments of ≤512MiB are ever
in flight, so the real high-water was ~2GiB and `awaitQuota` could not fire at the 8GiB
default. The advertised backpressure was not the one that ran.

The bounded channel becomes an unbounded queue (slice plus condition variable) drained by
the applier. Queue entries are pointers, so the queue itself costs nothing; what bounds
the system is `--spool-max-size` and only that. It also drops the decoupling window from a
fixed ~9s of database stall to whatever the operator's disk budget buys, which is what the
flag implied all along.

Two accounting fixes go with it, both of which let the quota mean what it says:

- `awaitQuota` runs after `seal()` has already written the files. Check the quota before
  writing, not after.
- `bytesOnDisk` excludes the open segment (`BytesOnDisk()` adds it for reporting only), so
  the real high-water is `MaxBytes` plus up to one segment. Count the open segment.

### Sealing on idle

A segment is written when it is big enough — or when the stream goes quiet. This is the
ordinary size-or-idle batching rule (Kafka's `linger.ms`, group commit, Nagle): the size
trigger buys throughput, the idle trigger bounds what is lost when the producer stops.

Today the triggers are size, `Close`, range completion (`sinker.go:192`) and the live
switch (`postgres/database.go:203`) — nothing covers a stream that simply stalls
mid-backfill. The open segment then sits unsealed indefinitely, the cursor never advances,
and those blocks are streamed and paid for a second time on restart.
`--spool-max-idle` fills exactly that hole and no other.

**Idle, not a deadline.** An alternative would be a maximum interval between commits, but
that fires hardest on the case it is not for: a slow-but-progressing stream would have its
segments chopped short precisely when it can least afford small commits. Idle is
self-disabling under load — it can only fire when there is nothing to lose by sealing
early — which is what makes a 10s default safe.

**Trigger is rows, not blocks.** "No new row written to the open segment", so a chain
producing blocks whose module output is empty still counts as idle.

Two things this deliberately does not solve:

- **A trickle is not idle.** Rows arriving steadily but slowly never trip the timer, so
  the segment fills over a long stretch and a crash re-streams all of it. That is the case
  a deadline would catch. Skipped: backfill throughput is high by definition, a sparse
  module produces rows in bursts far enough apart that idle does fire, and the residual
  blast radius is one segment. Revisit if it bites.
- **Rows arriving just slower than the window** — every 11s against a 10s idle — make every
  row its own segment and its own transaction. At that arrival rate the database is doing
  one tiny transaction every 11s. Not worth guarding.

**It needs a mutex, and that is its real cost.** `b.current` is touched only from the
sinker's goroutine today and is unsynchronized. Idle sealing has to come from a timer
goroutine — when the stream stalls the sinker is blocked in a read and has no next
opportunity to check anything — so the open segment needs a lock shared with
`Insert`, `RecordBlock`, `RecordCursor` and `MaybeSeal`.

## Command surface

No `run` subcommand — the root `sink postgres [<manifest> [<module>]]` stays the run
action.

```
sink postgres [<manifest> [<module>]]      run
sink postgres setup <manifest>             create the schema
sink postgres constraints apply <manifest> create constraints on a loaded database
sink postgres constraints drop <manifest>  drop them again
sink postgres tools ...                    unchanged
```

`apply-constraints` becomes `constraints apply`. `constraints drop` is new: the escape
hatch after `--apply-constraints=always`, and what makes a stalled backfill fast again
without a re-`setup`.

Persistent flags on `sink postgres` are unchanged: `--dsn`, operator flags. Everything
below is registered non-persistently with `.Flags()`, so no subcommand inherits a knob it
cannot act on — and `addFromProtoModeRunFlags` stops being called from `setup` and from
the constraints commands.

### Root command (run) — added

| flag | type | default | description |
|---|---|---|---|
| `--write-mode` | string | `auto` | How a sealed spool segment reaches the database: `copy` (binary COPY), `batch-insert` (one multi-row INSERT per table), `row-insert` (one prepared INSERT per row). `auto` picks copy on PostgreSQL, batch-insert on ClickHouse, and row-insert for a schema whose foreign keys cannot be ordered. An explicit mode the driver or the schema cannot support is an error, not a downgrade. Ignored once the stream reaches chain HEAD. |
| `--decode-workers` | int | `0` | Blocks unmarshalled and walked concurrently. Zero takes one per core less one for the goroutine draining the stream, capped at 8: measured at 4.13x on eight workers and only 4.24x on fifteen, the work being allocator-bound well before it runs out of cores. CPU only — does not change what the database sees. |
| `--decode-batch-size` | int | `0` | Blocks held in memory and decoded together. Zero takes four per decode worker. Larger keeps the workers fed; smaller costs less memory, since every held block keeps its payload and its decoded rows. Sizes the CPU stage, not the database write. |
| `--db-write-target-duration` | duration | `3s` | How long one commit to the database should take. A commit is one spooled segment, whichever write mode applies it; each is measured and the next segment sized toward this. Raise it for fewer, larger commits; lower it to keep the sink from occupying a database shared with something else. Backfill only. |
| `--db-write-max-size` | size | `512MiB` | Ceiling for the segment size the sizer may choose, whatever the target duration would allow. Backfill only. |
| `--spool-dir` | string | `./localdata/spool` | Directory pending segments are written to. The stream never waits on the database, and blocks already downloaded survive a restart instead of being streamed, and paid for, twice. Backfill only. |
| `--spool-max-size` | size | `8GiB` | Disk budget for `--spool-dir`, and the only bound on how far ahead of the database the stream may run. The stream is held once pending segments reach it, which is what turns a slow database into backpressure rather than a full disk. Backfill only. |
| `--spool-max-idle` | duration | `10s` | Write the open segment to the database once no new row has reached it for this long, short of its size target. A stream that stalls would otherwise sit on those rows indefinitely, leaving the cursor where it was and the blocks to be streamed, and paid for, again on restart. Zero disables it. Backfill only. |

### Root command (run) — changed

| flag | change |
|---|---|
| `--apply-constraints` | values become `auto` (default), `manual`, `always`. `auto` has the sink create the constraints itself once the backfill reaches chain HEAD, or at the end of a bounded run. `manual` leaves it to `sink postgres constraints apply`. `always` creates them before the first row and loads with them in place — measured through binary COPY, 27x slower than loading without, where building the same constraints afterwards costs 3.3x. |

Old values map: `head` → `auto`, `manual` → `manual`, `upfront` → `always`. They are
renamed outright with no alias, `--apply-constraints` never having shipped.

**The default changes from `manual` to `auto`, deliberately.** A sink that reaches HEAD
with no primary keys and no foreign keys has produced a database nobody should query, and
leaving it that way until the operator remembers to run a second command is the worse
failure — it is silent, and it looks like success. `auto` accepts a stop-the-world pass at
the end of the backfill as the price of a database that is correct when the sink says it
is done. Operators who need that pass inside a maintenance window ask for `manual`, which
is exactly what the flag is for.

What `auto` owes the operator, since the stall is real:

- A log line **before** it starts, at Info, naming what is about to happen, that the
  tables are locked while it runs, and that `--apply-constraints=manual` is how to take
  the pass into a maintenance window instead.
- Per-constraint progress as they are built, so a long pass is distinguishable from a
  hung one.
- The elapsed time when it finishes.

Unchanged on the root command: `--disable-foreign-keys`, `--disable-primary-keys`,
`--disable-unique-constraints`, `--proto-file-override`, `--bytes-encoding`.

### Root command — deprecated

A deprecation alias is owed only to a flag that has actually shipped. Everything the local
buffer work added is unreleased as of `v1.21.0` — `--apply-constraints`,
`--disable-foreign-keys`, `--disable-primary-keys`, `--disable-unique-constraints`,
`--local-buffer`, `--local-buffer-max-size` all arrived on
`feature/sink-sql-local-cache` and have never been in a tag. Those are renamed outright,
with no alias and no warning: nobody can have a script using them.

Only these two are in `v1.21.0` and get an alias, honoured for one release with a warning
naming the replacement:

| old | maps to |
|---|---|
| `--block-batch-size` | `--decode-batch-size` |
| `--no-constraints` | `--disable-foreign-keys --disable-primary-keys=all --disable-unique-constraints=all` |

`--no-constraints` needs the alias for a second reason: **the local buffer branch deletes
it without a replacement path.** It is registered and read on `develop`
(`sink_sql_common.go:156`, `:294`, `:496`) and is gone entirely on the branch. It maps
exactly onto `sql.DisableAllConstraints()`, so restoring it as an alias is mechanical, and
it must land whether or not the rest of this plan does.

Renamed with no alias (unreleased): `--local-buffer` → `--spool-dir`,
`--local-buffer-max-size` → `--spool-max-size`, `--apply-constraints` values
`head`/`upfront` → `auto`/`always`. The old `--local-buffer ""` convention for turning
buffering off disappears with no equivalent — the spool is always on.

### `setup <manifest>` — added: none

Subtractive. Keeps `--disable-foreign-keys`, `--disable-primary-keys`,
`--disable-unique-constraints` (they shape the DDL this command writes),
`--bytes-encoding` (decides column types), `--proto-file-override` (resolves the schema),
plus its existing DatabaseChanges-mode flags.

Drops `--apply-constraints` (timing is a run-time decision), `--block-batch-size`,
`--local-buffer`, `--local-buffer-max-size`. Does not register any `--write-mode`,
`--decode-*`, `--db-write-*` or `--spool-*`.

New behaviour: `setup` records the effective constraint policy in the sink-info table so
the run and the constraints commands can read it back.

### `constraints apply|drop <manifest>` — added: none

Keeps `--disable-foreign-keys`, `--disable-primary-keys`, `--disable-unique-constraints`
— now *overrides* on top of what `setup` recorded, with a warning naming both values when
they disagree — plus `--bytes-encoding` and `--proto-file-override`.

Drops everything else. The common invocation becomes
`sink postgres constraints apply <manifest> --dsn ...`.

`constraints drop` takes the same flags and removes the primary keys, unique constraints
and foreign keys the policy describes, skipping the ones already absent. Both are
idempotent.

## DatabaseChanges mode owns its schema

A Substreams whose output module is `sf.substreams.sink.database.v1.DatabaseChanges` (or
the older `sf.substreams.database.v1.DatabaseChanges`) does not use the from-proto model
at all. The operator owns the SQL schema entirely: it comes from the `schema.sql` bundled
in the manifest, the sink never derives tables from a proto descriptor, and there is no
constraint policy for the sink to have an opinion about. Mode is detected from the output
module type at run time (`sink_sql_common.go:242`, `:467`), not from a flag.

Everything in this plan is from-proto only. Today the from-proto flags are silently
ignored in DatabaseChanges mode; that becomes an error.

- **`constraints apply` and `constraints drop` fail outright** when the resolved output
  module is DatabaseChanges:

  > this Substreams outputs DatabaseChanges, where the SQL schema is yours: it is created
  > from the `schema.sql` bundled in the manifest, and the sink neither derives it nor
  > manages its constraints. `sink postgres constraints` only applies to from-proto
  > Substreams.

- **From-proto flags explicitly set in DatabaseChanges mode fail**, on the root command
  and on `setup`: `--write-mode`, `--decode-*`, `--db-write-*`, `--spool-*`,
  `--apply-constraints`, `--disable-*`, `--proto-file-override`. The check is on
  `cmd.Flags().Changed(name)`, so a default value never trips it — only a flag the
  operator typed.

  > `--spool-max-size` only applies to from-proto Substreams. This one outputs
  > DatabaseChanges, where rows are written from the module's own database changes and
  > the schema is the `schema.sql` in the manifest.

- **The reverse fails too.** `warnIgnoredDatabaseChangesSetupFlags`
  (`sink_sql_common.go:532`) already covers DatabaseChanges-only setup flags passed to a
  from-proto setup, but it only warns. Promote it to the same error, on the same
  `Changed()` test, and extend it from `setup` to the run command:
  `--batch-block-flush-interval`, `--batch-row-flush-interval`,
  `--live-block-flush-interval`, `--flush-retry-count`, `--flush-retry-delay`,
  `--undo-buffer-size`, `--cursors-table`, `--history-table`.

  Both directions error for the same reason: a flag typed for the other mode means the
  operator expects the sink to be doing something it will not do, and a warning in a log
  that scrolls past is not how they find that out.

The detection runs after the package and module are resolved, so the guard belongs in
`newSinkRunE` and `newSinkSetupE` at the point they branch, and at the head of the
constraints commands' `RunE`.

### Both vocabularies live on one command

Mode is detected from the module, not chosen by a flag, so a flag cannot be registered
conditionally — the command has not read the package yet when `init()` runs.
`addSinkRunFlags` (`sink_sql_common.go:204`) therefore registers both
`addDatabaseChangesModeRunFlags` and `addFromProtoModeRunFlags` on the same root command,
and that is not fixable.

The result is that `substreams sink postgres --help` lists both batching vocabularies
side by side — `--batch-block-flush-interval`, `--batch-row-flush-interval`,
`--live-block-flush-interval` next to `--decode-batch-size`, `--db-write-*` and
`--spool-*` — with a `[mode]` prefix per line as the only thing separating them. Removing
the from-proto flags from `setup` and the constraints commands does not help here; the run
command still shows every flag, half of them inert for any given Substreams.

So partition the help output instead: a custom usage template rendering
`From-proto mode flags:`, `DatabaseChanges mode flags:` and `Common flags:` as separate
sections. That turns a per-line tag the operator has to scan into a heading they can skip.
The `[from-proto mode]` / `[DatabaseChanges mode]` prefixes then come out of the
individual help strings, which shortens every one of them.

Grouping and the symmetric error are complementary: the sections say which half applies,
and typing from the wrong half fails rather than being ignored.

## Messages that replace silence

- At startup: one line with the resolved write mode, why it was resolved that way, and the
  effective value of every knob that applies.
- `--write-mode=copy` (or `batch-insert`) with an unorderable foreign key graph: **error**,
  naming the fix (`--disable-foreign-keys`). `auto` keeps the fallback but says which mode
  it picked and why.
- `--apply-constraints=always` with any write mode: **warn**, that being the measured 27x
  path — chosen deliberately or by accident, and the log line is the only thing that tells
  them apart.
- At the live switch: the existing message (`postgres/database.go:209`) gains the second
  half — which flags stop applying from here on.

## Decisions worth keeping

- **`--apply-constraints=auto` is the default**, stop-the-world pass and all. A backfill
  that ends with no constraints is a silent wrong result; a stall is a visible one. The
  pass is at least visible.
- **Spool format is per-mode and pre-rendered**: binary COPY for `copy`, rendered tuples
  for `batch-insert`, an interleaved log for `row-insert`, typed values for ClickHouse.
- **One release of deprecation, and only for flags that have shipped** — `--block-batch-size`
  and `--no-constraints`. Everything this branch added is renamed outright, none of it
  having been in a tag.
- **One sizer dial: segment bytes, in every mode.** No rows-based flag, so
  `--db-write-max-size` keeps one meaning. `row-insert` needs no special case — being
  slower per row, it just converges on a smaller segment at the same target duration.
- **No `--db-write-min-size`.** The lower clamp stays as an internal constant. It guards
  the controller against a death spiral; it is not a policy, and as a flag it would
  sometimes silently override the sizer it appears to configure.
- **No `--spool-max-chunks`.** `--spool-max-size` is the only ceiling on how far ahead of
  the database the stream may run.
- **`--spool-max-idle`, 10s, sealing on idle rather than on a deadline.** A spool concern,
  not a `--db-write-*` one: it does not size a commit, it stops the sink sitting on rows
  nobody is adding to. Rejected alternatives: `--db-write-max-interval` (fires hardest on
  the slow-but-progressing streams it is not for) and `--db-write-max-age` (reads as
  retention, and names the data's staleness rather than the guarantee).

### ClickHouse: the spool asks nothing new of it

ClickHouse has no transactions and keeps its cursor in a file, which looks like it should
force a new recovery model on it. It does not. What ClickHouse does today is:

1. accumulate rows in memory, in typed columnar builders;
2. send one columnar INSERT per table, with no transaction (`accumulator_inserter.go:562`);
3. write the cursor to `--cursor-file-path` afterwards (`click_house/database.go:363`).

So a crash between 2 and 3 already re-streams those blocks and re-inserts those rows.
At-least-once is the guarantee ClickHouse ships with, and the spool does not have to
improve on it — it only has to not make it worse.

Which it does not. The spool is a list of preprocessed blocks, nothing more:

- **Applying a segment is exactly step 2 followed by step 3.** Same INSERTs, same cursor
  file, same order, same window.
- **Recovery needs no bookkeeping table.** The cursor file already says what landed.
  Segments on disk whose cursor is at or behind it are dropped; the rest are replayed. A
  segment that was applied but whose cursor write was lost gets replayed and duplicates
  exactly the rows re-streaming it would have duplicated — parity, and it saves paying
  Substreams for them a second time.
- **No `_segments_` table, no deduplication setting, no cursor migration.**

One honest cost, and it is the same one PostgreSQL pays: the cursor now advances per
segment rather than per flush, so a crash re-does up to one segment's worth of work
instead of one flush's. `--db-write-target-duration` and `--spool-max-idle` are what bound
it.

What it needed was a **format**, not a recovery model. ClickHouse's inserts are typed and
columnar rather than SQL text, so `FormatTuples` is the wrong fit — rendering to literals
would change the insert path and its type handling. `FormatValues` stores the row values
tag-prefixed instead, and the applier decodes them straight back into the same column
builders the accumulator has always used, so what reaches the server is byte for byte what
an unspooled flush would have sent.

A `*timestamppb.Timestamp` is normalised to `time.Time` on the way in, the accumulator
taking either. A value of a type the accumulator cannot consume fails at spool time rather
than being silently mangled at apply time.

## Outcome

Every flag above is registered, the three PostgreSQL write modes and ClickHouse all spool,
and `constraints apply|drop` reads the policy `setup` recorded. Principle 3 holds on both
drivers.

Two things the first draft of this plan got wrong, kept here because the reasoning is
worth having:

- **`row-insert` cannot spool by table.** The spool groups rows into one file per table,
  and a cyclic foreign key graph — the only reason that mode exists — has no table order
  that keeps a parent ahead of its children. Hence `FormatRowLog`, an interleaved file
  replayed in the walk's own order.
- **ClickHouse needed no new guarantees.** An earlier draft gave it
  `insert_deduplication_token` and a cursor table, which solved a problem the sink does
  not have: it already re-streams and re-inserts on a crash between the inserts and the
  cursor write. See the section above.
