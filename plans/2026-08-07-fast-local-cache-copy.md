# Local cache + binary COPY for the from-proto Postgres sink

Status: proposal, revised after measurement
Scope: `sink/sql/db_proto/**` (from-proto mode, PostgreSQL, insert-only). DatabaseChanges
mode and ClickHouse are out of scope.

Two goals, in priority order:

1. **Get data out of Substreams as fast as possible.** The user pays for Substreams
   throughput. The stream must never wait on PostgreSQL, and blocks already paid for
   must never be downloaded twice.
2. **Reach maximum end-to-end performance with very few knobs.** Everything that can be
   auto-tuned from a measurement should be, and the sink should always be able to say
   *which* of the three stages is the limiter.

## 1. What the measurements say

Two benchmark suites in `sink/sql/db_proto/benchmarks/`. All numbers are
`postgres:17-alpine` in a testcontainer on an M-series laptop; ratios matter, absolutes
do not.

### Server side — `TestCopyVsInsert`, 250k rows, 13 columns, ~88 MiB

| strategy | duration | rows/s | vs current |
|---|--:|--:|--:|
| per-row prepared INSERT (`RowInserter`) | 22.199s | 11k | 0.10x |
| multi-row INSERT built at flush (`AccumulatorInserter`, current) | 2.281s | 110k | 1.00x |
| multi-row INSERT prebuilt on disk | 1.989s | 126k | 1.15x |
| COPY CSV from disk | 724ms | 345k | 3.15x |
| **COPY BINARY from disk** | **313ms** | **799k** | **7.29x** |
| `pgx.CopyFrom` over in-memory rows | 339ms | 737k | 6.73x |

### Client side — `TestClientEncodeCeiling`, single core, no database

Production path: `proto.Unmarshal` into `dynamicpb`, then
`WalkMessageDescriptorAndInsertWithDialect` with the real Postgres dialect.

| stage | narrow entity (2 cols) | wide entity (60+ cols) |
|---|--:|--:|
| unmarshal only | 1.79M rows/s | 203k rows/s |
| + walk (discard values) | 855k rows/s | 128k rows/s |
| + encode to binary COPY | **745k rows/s** | **96k rows/s** |
| + encode to text literals (current) | 767k rows/s | 108k rows/s |

Per wide row, single core: 4.9 µs unmarshal, 2.9 µs walk, 2.6 µs encode = 10.4 µs.

### The conclusion that reorders the whole plan

**For wide entities the sink's own CPU is the bottleneck, not PostgreSQL.** One core
produces 96k rows/s; binary COPY absorbs 799k rows/s. PostgreSQL has ~8x headroom that
the current single-threaded handler cannot use. For narrow entities the two are within
10% of each other.

So the ranked levers are:

1. **Parallelise decode + walk + encode across cores.** Blocks are independent. On 10
   cores a wide-entity workload goes from ~96k to ~900k rows/s, at which point
   PostgreSQL becomes the limiter again — which is the correct place for it to be.
2. **Binary COPY** — 7.3x on the server side, and the only strategy that can absorb the
   parallelised client.
3. **Decouple the stream from the flush** — so a slow or stalled PostgreSQL never stops
   the download the user is paying for.
4. Pre-encoding to disk is worth only ~8% of raw throughput on its own (313ms vs 339ms).
   Its value is entirely in (3) and in never re-downloading.

### Two hypotheses tested and rejected

- **"NUMERIC encoding via big.Int is the hot spot."** Mapping every unsigned integer to
  `BIGINT` instead of `NUMERIC` moves wide-entity throughput from 96k to 100k rows/s,
  4%. Not worth a schema change or a hand-rolled numeric encoder. (`uint32` →
  `NUMERIC` at `sql/postgres/types.go:75` is still arguably wrong on its own merits,
  since `uint32` fits `BIGINT` exactly — but it is not a performance argument.)
- **"Building SQL strings is the current bottleneck."** Prebuilt-on-disk statements are
  only 1.15x faster than building them at flush time, so `ValueToString` plus
  `strings.Builder` are ~13% of the current cost. The other ~87% is the server parsing
  a multi-megabyte statement. Optimising the Go-side string building is a dead end.

One optimisation was found and kept: caching the pgtype encode plan per column in
`pgcopy.Writer` rather than letting `Map.Encode` resolve one per value. Worth +8% on
wide entities, +3% on narrow, no downside.

## 2. Architecture

```
 substreams gRPC stream
        │  raw BlockScopedData, handler returns immediately
        ▼
  [block queue, bounded]
        │
        ├──▶ decode worker 1 ─┐   unmarshal (dynamicpb)
        ├──▶ decode worker 2 ─┤   walk message descriptor
        └──▶ decode worker N ─┘   encode to PGCOPY binary
        │
   [segment slots, filled in stream order]
        │
        ▼
  local cache: sealed segments on disk        ◀── download cursor lives here
        │  (PGCOPY binary + manifest.json)
        │
        ├──▶ COPY worker 1 ─┐
        └──▶ COPY worker M ─┘  COPY ... FROM STDIN (FORMAT BINARY)
                    │
                    ▼
                PostgreSQL                    ◀── apply cursor lives here
```

Each stage is bounded and instrumented, which is what makes the bottleneck verdict in §6
possible.

### 2.1 Two cursors

This is the change that serves goal 1 directly.

- **Download cursor** — persisted in the local cache next to the segments. On restart the
  stream resumes from here.
- **Apply cursor** — in PostgreSQL, in `_cursor_`, as today. Only ever advances to the
  last block of a committed segment.

Today there is only the apply cursor, so a PostgreSQL outage or a sink crash means
re-streaming — and re-paying for — every block since the last successful commit. With a
local cache, downloaded blocks are on disk and are never fetched twice. If the cache is
lost or torn, the sink falls back to the apply cursor and re-streams, which is correct,
just slower and billable.

### 2.2 Ordering with parallel decode

Rows are insert-only with no foreign keys in this mode, so the *data* has no ordering
requirement. Only the segment→cursor mapping does.

Assign each block a slot in the current segment at receive time, in stream order. Workers
fill their slots in any order. A segment seals when every slot in it is filled, and its
manifest records the cursor of its highest block. No reorder buffer, no per-row locking.

### 2.3 Segment layout

```
<local-cache>/<schema>/
    download-cursor.json                       # cursor + block of the last sealed segment
    seg-%016d-%s/                              # first block num + random suffix
        manifest.json                          # written last; its presence = sealed
        <table>.pgcopy                         # PGCOPY binary, one file per table
```

`manifest.json` holds `first_block`, `last_block`, `cursor`, and per table the file name,
row count, byte count and the resolved column list. A segment without a valid sealed
manifest is torn and is deleted on recovery.

### 2.4 Why PGCOPY binary on disk

`COPY ... FROM STDIN (FORMAT BINARY)` is a length-prefixed tuple stream. Writing that
exact format to disk makes the flush an `io.Copy` from file to socket: no re-encoding, no
escaping, no parsing at flush time. That is what lets the COPY workers run independently
of the decode workers, and it makes the cache directory a first-class artifact that can
be loaded into any PostgreSQL later (§7).

Format: 19-byte header (`"PGCOPY\n\377\r\n\0"`, int32 flags, int32 header extension
length), then per tuple an int16 field count and per field an int32 length (`-1` for
NULL) plus the raw binary value, then an int16 `-1` trailer. Implemented in
`sink/sql/db_proto/sql/postgres/pgcopy`.

## 3. Column OIDs — the main correctness hazard

Binary COPY performs **zero type coercion**. The bytes for column *i* must match that
column's actual type OID exactly or PostgreSQL aborts the COPY, sometimes with a
confusing error. Text COPY would coerce; binary will not.

Do not derive OIDs from `MapFieldType`'s declared type names. Read them from the live
catalog once per table (`pgcopy.LoadColumns` does this against `pg_attribute`).

Conversions that need care, all handled by `pgcopy.Normalize`:

| Go value from the walker | Column type | Encoding |
|---|---|---|
| `uint64` | `NUMERIC` | `pgtype.Numeric` from `big.Int`; **never** send an int8 |
| `string` with an int128/uint256/decimalN `ConvertTo` | `NUMERIC`/`DECIMAL(p,s)` | parse to `pgtype.Numeric`; empty string stays 0, as `RowInserter` does today |
| `[]byte` | `BYTEA` / `TEXT` | raw bytes, or `bytesEncoding.EncodeBytes` when `IsStringType()` |
| `*timestamppb.Timestamp`, `time.Time` | `TIMESTAMP` | µs since 2000-01-01, not RFC3339 |
| `[]any` | `<base>[]` | typed slice, then the pgtype array codec |
| inline message | `JSONB` | `protojson.Marshal`, with the JSONB `0x01` version byte |

`TestPgCopyBinaryRoundTrip` loads 1000 rows across all 13 benchmark column types through
both a parameterised INSERT and binary COPY and asserts the results are byte-identical.
That test is the safety net; extend it whenever a new column type becomes reachable.

## 4. Auto-tuning: what the knobs are, and what they are not

Three flags, and nothing else user-facing:

```
--local-cache <path>             enable the decoupled path and say where the cache lives
--local-cache-max-size <size>    disk quota (default: min(10% of free space, 16GiB))
--local-cache-only               download only, never connect to PostgreSQL (see §7)
```

Everything else is derived at runtime:

| Quantity | How it is chosen |
|---|---|
| decode workers | `max(1, NumCPU-1)`; blocks are independent so this is free parallelism |
| COPY workers | start at 1; add one whenever the segment queue is non-empty **and** the previous addition improved segment throughput; drop back when it does not. Cap at 8 |
| segment size | target a 3s COPY; after each segment `next = clamp(current * clamp(3s/observed, 0.5, 2), 8MiB, 512MiB)`. Seal on size or a 30s age bound, never mid-block |
| direct vs cached | `isLive` — live blocks go through the existing accumulator, backfill goes through the cache (§8) |
| backpressure | block the receive loop when the queue is full or the disk quota is reached; count the wait |

Sizing segments by **bytes, not blocks or rows**: block payload size varies by orders of
magnitude across chains and modules, so a block count gives wildly unstable flush
durations. Multiplicative control with a 2x per-step clamp avoids oscillation.

## 5. Concurrency and transaction boundaries

The COPY workers apply whole segments. With more than one worker, segments can commit out
of order, so "applied through block N" stops being expressible as a single number.

Add a small `_segments_` table:

```sql
CREATE TABLE <schema>._segments_ (
    first_block BIGINT NOT NULL,
    last_block  BIGINT NOT NULL,
    cursor      TEXT   NOT NULL,
    applied_at  TIMESTAMP NOT NULL DEFAULT now(),
    PRIMARY KEY (first_block)
);
```

Each COPY worker runs one transaction per segment: COPY every table file, insert the
`_segments_` row, commit. The apply cursor in `_cursor_` is advanced by whichever worker
completes a segment that extends the contiguous prefix. Recovery replays any local
segment with no `_segments_` row.

This keeps exact atomicity per segment — a crash never leaves a partially applied
segment, so there is nothing to clean up and no `DELETE ... WHERE _block_number_ > x`
sweep is needed. Within one segment the table files are COPYed sequentially, because a
transaction is bound to one connection; parallelism comes from running several segments
at once, not several tables.

`_segments_` is also directly queryable by the user, which is half of goal 2.

## 6. Observability: naming the bottleneck

The sink must be able to say which stage is the limiter. All three states are real, and
the measurements in §1 show the third is the common one today.

| Signal | Verdict |
|---|---|
| block queue empty, decode workers idle | **Substreams is the limiter** — the sink is keeping up, the stream is as fast as it gets |
| block queue full, decode workers saturated, segment queue empty | **The sink's CPU is the limiter** — raise decode workers, or the entity is expensive to walk |
| segment queue growing, disk usage climbing, backpressure engaged | **PostgreSQL is the limiter** — the download is still running at full speed and buffering to disk |

Emit this as a single periodic log line plus Prometheus gauges, in the vocabulary the
user actually cares about:

```
downloaded through #20,431,200 (3.2 GiB on disk, 41k blocks ahead)
applied     through #20,390,118
limiter: postgres     copy 118 MiB/s, 4 workers, backpressure 62% of the last 30s
```

Metrics to add to `db_proto/stats.Stats`: `DecodeDuration`, `EncodeDuration`,
`CopyDuration`, `CopyBytes`, `BlockQueueDepth`, `SegmentQueueDepth`, `DiskBytesInUse`,
`BackpressureWaitDuration`, `DownloadedBlock`, `AppliedBlock`. `BackpressureWaitDuration`
near zero versus dominant is the single number that separates state 2 from state 3.

## 7. The cache as an artifact

Once the cache directory holds PGCOPY binary plus manifests, three things become
possible for free, and they serve goal 1 better than any tuning:

- **`--local-cache-only`** — stream and encode at full speed, never open a database
  connection. The user pays Substreams once, at the maximum rate their CPU allows.
- **`substreams sink postgres cache-load <dir>`** — load a cache directory into any
  PostgreSQL, with as many parallel COPY workers as that server can take. Offline,
  resumable, and repeatable into several databases.
- **`substreams sink postgres cache-status <dir>`** — report what has been downloaded
  without touching PostgreSQL at all.

`cache-load` overlaps heavily with the existing `inject-csv` command
(`cmd/substreams/sink_postgres_inject_csv.go`), which already streams CSV into
`PgConn().CopyFrom`. Same shape, binary format, manifest-driven.

## 8. Live blocks, undo, and constraints

- **Live blocks bypass the cache.** When `isLive`, use the existing accumulator path:
  latency matters, volume is trivial, undo signals arrive, and round-tripping through
  disk is pure overhead. Backfill uses the cache. The transition backfill→live must
  **drain the segment queue and wait for the last COPY to commit** before the first
  direct insert, or a direct row could land ahead of an older cached one.
- **Undo signals** only occur near head, where the queue is empty. Guard explicitly:
  assert an empty queue and no in-flight segment, then run the existing
  `HandleBlocksUndo`. Error loudly rather than silently mis-ordering.
- **The cached path requires `--no-constraints`.** `useConstraints` already selects
  `RowInserter`, a different path entirely, and FK checks are immediate per row unless
  declared `DEFERRABLE INITIALLY DEFERRED`, which per-table COPY files would violate.
  Fail fast at startup if both are set.

## 9. Crash recovery

On startup, before running the stream:

1. Read the apply cursor from PostgreSQL and the applied ranges from `_segments_`.
2. Scan the cache for segment directories, sorted by `first_block`.
3. Drop segments without a valid sealed manifest (torn write).
4. Drop segments already recorded in `_segments_` (crash between COMMIT and `rm`).
5. Drop segments beyond a hole — if an earlier segment was lost, nothing after it is
   usable.
6. Replay the contiguous remainder through the COPY workers.
7. Resume streaming from the download cursor, which is now consistent with what is on
   disk.

**fsync policy:** fsync only `manifest.json` on seal, not the data files. A process crash
is fully covered. A machine crash may leave a sealed segment short, so validate each data
file's trailer and byte length against the manifest at recovery; on mismatch discard from
that segment onward and fall back to the apply cursor. The cost of being wrong is
re-downloading a few segments — correct but billable, never corrupt.

## 10. Implementation order

1. `pgcopy` package: binary writer, OID resolution from `pg_attribute`, `Normalize`,
   round-trip test per column type. **Done** — `sink/sql/db_proto/sql/postgres/pgcopy/`.
2. Benchmarks establishing the server and client ceilings. **Done** —
   `sink/sql/db_proto/benchmarks/`.
3. **Parallel decode workers.** The largest single lever (§1). **Done** —
   `sink/sql/db_proto/decoder.go`. Measures 4.13x at eight workers on a wide entity
   (129k → 534k rows/s); scaling flattens past eight, the work being allocator-bound
   before it runs out of cores, so the default is capped there.
4. Cache writer: segments, manifests, download cursor, sealing. Synchronous COPY on seal
   at first, so the file format and recovery are exercised before concurrency is added.
5. COPY workers, `_segments_` table, bounded queues, disk-quota backpressure.
6. Auto-tuning of segment size and COPY worker count (§4).
7. Recovery and replay (§9), with a test that SIGKILLs mid-backfill and asserts no gaps
   and no duplicates.
8. Bottleneck verdict logging and metrics (§6).
9. `--local-cache-only`, `cache-load`, `cache-status` (§7).

Step 3 is worth doing first and on its own: it needs no new file format, no recovery
story, and on the measured numbers it is where the throughput is.

## 11. Pre-existing issues in the way

These interact with adding worker goroutines and must be fixed or consciously accepted
before step 3:

- ~~`holding` is a package-level global, `Database.Clone()` does not clone, and
  `appengine.MultiError` is appended to from several goroutines without a lock.~~ Fixed:
  the unreachable `--parallel` path that depended on all three is removed.
- ~~`postgres.ValueToString` renders raw bytes as `E'<hex>'::BYTEA`, storing twice the
  intended bytes.~~ Fixed on its own branch, `fix/sink-sql-bytea-encoding` — unrelated
  to this work.
- Blocks still held when a bounded run reaches its stop block are never flushed: nothing
  drains `holding` at stream end. Not touched here, but it bounds what any test of the
  flush path can assert.
- ~~`WalkMessageDescriptorAndInsertWithDialect` builds `zap.Any` debug fields once per
  message whether or not debug is enabled.~~ Fixed: both that and the per-row equivalent
  in `RowInserter` are behind `Check` now. Worth ~6% on a narrow entity.

## 12. Server-side settings worth trying first

None of this removes WAL amplification, index maintenance, checkpoint I/O or constraint
checks. With two secondary indexes on the benchmark table, binary COPY's advantage drops
from 7.29x to 3.84x. Before assuming the ratios above transfer:

```sql
SELECT query, calls, total_exec_time, mean_exec_time
FROM pg_stat_statements ORDER BY total_exec_time DESC LIMIT 20;

SELECT wait_event_type, wait_event, count(*)
FROM pg_stat_activity WHERE state='active' GROUP BY 1,2;
```

Cheap wins that may reduce how much of this is needed:

- `synchronous_commit = off` on the sink's session — safe here, because a lost commit is
  recovered by the cache replay logic.
- Raise `max_wal_size` and `checkpoint_timeout` for the duration of a backfill.
- Drop secondary indexes for the backfill and build them at the end. Usually the single
  largest factor at tens of GB.
