---
description: Learn how to read the progress and performance output the SQL sink logs while it runs
---

# Progress & Performance Output Reference

A running `substreams-sink-sql` prints two independent things on a loop:

- a **one-line stream summary**, `substreams stream stats`, emitted by the sink library
  ([`sink/stats.go`](https://github.com/streamingfast/substreams/blob/develop/sink/stats.go)) every 15s (5s in development mode). It describes the
  *stream*: what the Substreams backend is producing and what has arrived.
- a **multi-line panel**, framed by `-----------------------------------`, emitted by the
  proto sink every 30s ([`sink/sql/db_proto/stats/stats.go`](https://github.com/streamingfast/substreams/blob/develop/sink/sql/db_proto/stats/stats.go)).
  It describes the *sink*: where the time went between a block arriving and its rows being
  committed to the database.

They are never in sync, and they measure different ends of the pipeline. Read the one-liner
to answer "am I being fed fast enough", and the panel to answer "what am I doing with what
I am fed".

```
INFO (substreams) substreams stream stats {"data_msg_rate": "5386.833 msg/s (246075 total)", ...}
INFO (substreams) -----------------------------------
INFO (substreams) Stats {"block_count": 264390, ...}
INFO (substreams)    Wait Duration Between Blocks {...}
INFO (substreams)       Block Processing Duration {...}
INFO (substreams)          Unmarshalling Duration {...}
INFO (substreams)            Spool Write Duration {...}
INFO (substreams)           Message Walk Duration {...}
INFO (substreams)                  Flush duration {...}
INFO (substreams)               Pipeline progress {...}
INFO (substreams)                   Spool applier {...}
INFO (substreams)                Spool throughput {...}
INFO (substreams)                  Spool pressure {...}
```

The panel's lines are right-aligned on purpose: the ladder from `Wait Duration Between
Blocks` down to `Flush duration` is one column of comparable numbers, and the indent is
what makes them comparable at a glance.

---

## 1. `substreams stream stats` — the stream side

```
{"data_msg_rate": "5386.833 msg/s (246075 total)", "undo_msg_rate": "0.000 msg/s (0 total)",
 "avg_block_wait_time_sec": 0.007261055, "avg_block_time_delta_sec": 16.905561848,
 "avg_local_block_processing_time_sec": 0.000000087,
 "last_block": "#22112999 (840dcf64…)",
 "progress_block_rate": "3200.000 block/s (457671 total)",
 "progress_last_block": {"stage 0":22295000}, "progress_running_jobs": {"stage 0":150},
 "progress_total_processed_blocks": 457671, "is_live": false}
```

| Field | Meaning | How to read it |
|---|---|---|
| `data_msg_rate` | `BlockScopedData` messages received per second, 30s rolling average, with the lifetime total in parentheses. | This is the sink's actual input rate. If it is high and the panel says the database is behind, the database is the bottleneck. |
| `undo_msg_rate` | `BlockUndoSignal` messages per second, plus lifetime total. | Zero for the whole backfill; anything non-zero means reorgs are being handled. Every undo forces a full drain of the write path. |
| `avg_block_wait_time_sec` | Average time the sink sat blocked waiting for the *next* message from the stream (30s rolling). | Near zero (as here, 7µs) means the stream always has a block ready — the sink is the limiter, not the network. Once live this converges on the chain's block time. |
| `avg_block_time_delta_sec` | Average difference between consecutive blocks' *block timestamps*. | A chain property, not a performance number. It is heavily skewed when a block filter or skipped outputs mean whole ranges are not delivered — 16.9s here on a chain with much shorter blocks just means blocks are being filtered out. |
| `avg_local_block_processing_time_sec` | Average time spent inside the sink's message handler. | Misleadingly small on the proto sink: the real per-block cost is measured in the panel instead. Treat the panel's `Block Processing Duration` as authoritative. |
| `last_block` | Number and hash of the most recent block delivered to the sink. | This is what has been *downloaded*, not what has been committed. |
| `progress_block_rate` | Blocks per second the *backend's parallel engine* is processing, plus total. | Server-side work, in front of the stream. It can be far higher than `data_msg_rate` because parallel stages run ahead of the linear output you receive. |
| `progress_last_block` | Highest block reached, per stage. | Ahead of `last_block` during a backfill — that is the parallel engine running in front of you. |
| `progress_running_jobs` | Number of active backend jobs, per stage. | A steady, high count means the backend is fully parallelized. A count that collapses to a few means the backend has run out of work to schedule, and the stream rate is about to drop. |
| `progress_total_processed_blocks` | Total blocks processed by the backend for this request. | This is what gets billed, and it is not the same as blocks delivered. |
| `is_live` | Whether the stream is considered at the chain head. | `false` for a backfill. When it flips to `true` the proto sink switches away from the spool to direct inserts — many of the panel's lines change with it. |

The `progress_*` fields and `is_live` are only printed once liveness is known.

---

## 2. `Stats` — the run's totals

```
{"block_count": 264390, "Processing Time": "3.78s", "Total Wait Duration": "2m 16.83s",
 "Total Duration": "2m 20.61s", "Last Block Process At": "2026-08-24T08:35:38.525-0400"}
```

| Field | Meaning |
|---|---|
| `block_count` | Blocks that have reached the sink's handler in this run. |
| `Processing Time` | Cumulative wall clock spent *inside* the handler, with database-imposed waits removed. |
| `Total Wait Duration` | Cumulative wall clock spent waiting on the stream for the next block. |
| `Held By Database` | Cumulative time the stream was held because the spool's disk quota was full. **Only printed when it is non-zero.** Its absence above means the disk budget was never the limit. |
| `Total Duration` | `Total Wait Duration + Processing Time + Held By Database` — the run's accounted wall clock. |
| `Last Block Process At` | Timestamp of the most recent block processed. If this stops advancing while the process is alive, the sink is stalled. |

The split of `Total Duration` is the first diagnosis. In the example, 2m16s of 2m20s is
*waiting on the stream* and 3.8s is processing: this run is stream-bound, and no amount of
database tuning would help it.

`Held By Database` is deliberately kept out of `Processing Time`. Folding it in would make
"processing" grow precisely when the database slows down, while the wait between blocks
shrank to nothing — exactly backwards from what is happening.

---

## 3. The duration ladder

```
   Wait Duration Between Blocks {"per_1000_blocks": "0.34s", "recent_per_1000_blocks": "50.40ms"}
      Block Processing Duration {"per_1000_blocks": "14.10ms", "recent_per_1000_blocks": "10.56ms"}
         Unmarshalling Duration {"per_1000_blocks": "1.98ms", "recent_per_1000_blocks": "1.49ms"}
           Spool Write Duration {"per_1000_blocks": "9.35ms", "recent_per_1000_blocks": "6.88ms"}
          Message Walk Duration {"per_1000_blocks": "7.44ms", "recent_per_1000_blocks": "5.57ms"}
                 Flush duration {"per_1000_blocks": "15.31ms", "recent_per_1000_blocks": "15.31ms"}
```

**Every sample behind these numbers is one block's cost**, and it is scaled up by 1000
before printing. A per-block mean lands in tenths of a microsecond — a column of `0.02ms`
and `0.08ms` that nobody can weigh against each other or against a wall clock. Scaled, the
ratios are obvious. The field names carry the scale, so they change if the constant is
retuned.

- `per_1000_blocks` — mean over a long window (the last 250,000 blocks; last 1,000 samples
  for `Flush duration`).
- `recent_per_1000_blocks` — mean over the tail of that window (last 1,000 blocks; last 10
  samples for `Flush duration`).

Compare the two columns to see the trend. Here `recent` is consistently below the lifetime
mean, so the run is getting faster, not degrading. When `recent` climbs above the long
window, something has changed *now* — a bigger block range, a database under load, a disk
filling up.

| Line | What it measures |
|---|---|
| `Wait Duration Between Blocks` | Time between finishing one block and receiving the next. This is the stream, not you. Large here and small everywhere else = stream-bound. |
| `Block Processing Duration` | Total wall clock inside the handler for one block, **excluding** time held by the disk quota. It is *not* the sum of the three lines beneath it — those are measured on the decode workers, which run several blocks concurrently, so the parts can exceed the whole. |
| `Unmarshalling Duration` | Protobuf decoding of the module output. |
| `Message Walk Duration` | Walking the decoded message and building row values. Touches neither database nor spool, so it is named the same in both modes. |
| `Spool Write Duration` | Writing those rows into the open spool segment on disk. **Named `Block Insert Duration` when there is no spool** (direct inserts, i.e. at chain head), where it is the actual database insert. The title tells you which write path is live. |
| `Flush duration` | How long it took to get one block *into the database*. With a spool, the sink's own flush only queues rows, so this is derived from what the applier committed instead — which is what keeps this line meaning the same thing in both modes. |

If `Spool Write Duration` dominates, you are I/O-bound on the spool directory. If
`Unmarshalling` and `Message Walk` dominate, you are CPU-bound and `--decode-batch-size`
and worker count are what to look at. If `Flush duration` dominates, the database is the
limit.

---

## 4. `Pipeline progress` — the gap

```
{"downloaded_through": 22136999, "applied_through": 22065996, "blocks_ahead": 71003,
 "blocks_buffered": 70998, "peak_blocks_ahead": 195046, "buffered_on_disk": "55 MiB",
 "quota_used": "0.7%"}
```

This is the number that matters most. Substreams throughput is paid for, so a run should be
limited by the stream, not by the database — and when it is not, this is where it shows.

| Field | Meaning |
|---|---|
| `downloaded_through` | Highest block received from the stream. |
| `applied_through` | Highest block actually committed to the database. With a spool this comes from what the applier committed, not from what the sink flushed — queued is not durable. |
| `blocks_ahead` | `downloaded_through - applied_through`. The gap. |
| `blocks_buffered` | Blocks held in memory for the next flush, plus everything the spool has queued on disk. |
| `peak_blocks_ahead` | High-water mark of `blocks_ahead` for this run. |
| `buffered_on_disk` | Bytes the spool currently holds (sealed segments plus the open one). Omitted when nothing is on disk. |
| `quota_used` | `buffered_on_disk` as a share of `--spool-max-size`. |

`blocks_ahead` and `blocks_buffered` should track each other closely (71003 vs 70998 here);
the small difference is blocks in flight between the two counters.

**A gap on its own does not assign blame.** It grows both when the database is slow and when
the stream merely bursts. That is what the three `Spool …` lines below are for.

**This line becomes a `WARN` when the buffer stops looking like a working set:**

```
WARN database is falling behind the stream, the buffer is over half of what it was given
```

The threshold depends on the mode. With a spool it is the *disk budget* — the warning fires
once `buffered_on_disk` is at least half of `--spool-max-size`, because the budget is the
whole point of the spool and the operator chose it. Without a spool (or after the switch to
direct inserts closed it), the blocks are held in memory and the threshold is a block count:
four batches' worth, with a floor of 100.

At `0.7%` of quota, the example run is nowhere near it.

---

## 5. `Spool applier` — how commits are going

```
{"segments": 7, "rate": "2.0 seg/min", "blocks_per_seg": 51117, "rows_per_seg": 127420,
 "apply_duration": "0.23s", "queue_depth": 0}
```

The spool writes rows to disk in *segments*; a background applier goroutine loads each
sealed segment into the database. One segment is one commit.

| Field | Meaning |
|---|---|
| `segments` | Cumulative segments this process's applier has committed. Segments replayed by crash recovery are excluded — counting another run's work as this run's throughput would skew the rates. |
| `rate` | Segments committed per minute **during the last interval**, not over the run. |
| `blocks_per_seg` | Blocks per segment over the last interval. |
| `rows_per_seg` | Rows per segment over the last interval. |
| `apply_duration` | Mean time to commit one segment over the last interval. **This is what `--db-write-target-duration` steers** (default 3s) and what the segment sizer converges on. |
| `queue_depth` | Sealed segments waiting for the applier right now. |

`blocks_per_seg`, `rows_per_seg` and `apply_duration` are only printed when at least one
segment completed during the interval — otherwise there is nothing to average.

`apply_duration` of 0.23s against a 3s target means the sizer will keep growing segments.
A `queue_depth` that stays above zero means the applier cannot keep up with sealing; a
`queue_depth` of 0 with a large gap means the *stream* is producing faster than segments
seal, not that the database is slow.

Rates are differenced against the previous tick rather than divided by total run time: a
lifetime average says nothing once anything has changed, and would report a healthy rate
for a backfill that was fast for an hour and has been stalled for ten minutes.

---

## 6. `Spool throughput` — how much is moving

```
{"rows": "4,246/s (738,651 total)", "applied_rate": "1.2 MiB/s", "total_applied_bytes": "199 MiB"}
```

| Field | Meaning |
|---|---|
| `rows` | Rows committed per second during the last interval, with the run's cumulative total. |
| `applied_rate` | Bytes committed per second during the last interval. |
| `total_applied_bytes` | Cumulative bytes committed. |
| `db_write_rate`, `db_bytes` | Bytes that actually crossed the socket to the server — rate and total. **Only printed when the driver can measure it.** |

`applied_rate` and `total_applied_bytes` are sized **in the format the spool's codec wrote
on disk**, not on the wire. With `--write-mode=copy` the two roughly coincide, but a
rendered write mode wraps payloads in SQL statements, and ClickHouse re-encodes them
columnar and may compress them. `db_bytes` is the wire figure where a driver can report it —
pgx owns its connection, so the PostgreSQL path leaves these two fields out entirely rather
than print a zero that would read as a database receiving nothing.

Only committed segments are counted. Rows sitting in the open segment or the queue are not
in these numbers — they are in `Pipeline progress`.

---

## 7. `Spool pressure` — where the backpressure is

```
{"applier_busy": "1.0%", "quota_wait": "0.00ms", "segment_target": "447 MiB",
 "open": "55 MiB / 70993 blocks", "open_age": "11.14s",
 "sealed_by_size": 2, "sealed_by_idle": 5, "sealed_by_drain": 0, "sealed_by_close": 0}
```

| Field | Meaning |
|---|---|
| `applier_busy` | Share of the applier goroutine's wall clock spent applying, **over the last interval**. |
| `quota_wait` | **Cumulative** time the stream was held because the spool hit its disk quota. |
| `segment_target` | Segment size the sizer has currently converged on, moving toward `--db-write-target-duration`. Bounded by an internal ceiling and by `--spool-max-size`. |
| `open` | Bytes and blocks in the segment being written right now. |
| `open_age` | How long the open segment has been accumulating. |
| `sealed_by_*` | Cumulative count of segments by what closed them. |

**`applier_busy` is the answer to "is the database or the stream the limit."** An applier
busy essentially all the time is the ceiling. At 1.0% here, the database is idle almost
continuously and this run is unambiguously stream-bound.

**Any non-zero `quota_wait` means the database is gating download** — the stream was
literally paused because there was nowhere to put rows. That is the strongest signal in the
whole panel. It is a running total, so watch whether it grows between ticks, not its
absolute value.

The **seal-reason mix is a diagnosis**:

| Reason | Meaning |
|---|---|
| `sealed_by_size` | The intended path: the segment reached the size the sizer chose. |
| `sealed_by_idle` | The idle timer fired (`--spool-max-idle`, default 10s) and committed a short segment, because the stream had gone quiet. |
| `sealed_by_drain` | An undo forced everything queued to reach the database first. |
| `sealed_by_close` | Shutdown. |

A run dominated by `size` is healthy. A run dominated by `idle` — as in the example, 5 idle
against 2 size — is committing segments short of the target, which starves the sizer and
costs one round-trip per short commit. That is normal during a bursty backfill or a stream
that keeps pausing; it is worth attention if it persists while the stream is saturated.
A non-zero `drain` count means reorgs are forcing full drains.

---

## 8. `Spool (closed)` — after the switch to direct inserts

When the stream reaches the chain head, the sink stops spooling and inserts directly. Rates
over an applier that has stopped would read as one that has stalled, and "in flight" would
describe something that no longer exists, so the three `Spool …` lines are replaced by one:

```
Spool (closed) {"segments": …, "blocks": …, "rows": …, "total_applied_bytes": …,
                "applying": …, "quota_wait": …}
```

These are the run's totals from the backfill, retained after the spool closed — including
the final drain, which commits everything still queued at the moment of the switch. From
here on, `Flush duration` measures a real database flush and `Spool Write Duration` is
printed as `Block Insert Duration`.

---

## 9. Reading the whole panel: worked examples

**Stream-bound (the example above).** `Total Wait Duration` is 97% of `Total Duration`;
`applier_busy` is 1%; `quota_wait` is zero; `quota_used` is 0.7%. The database is barely
working. Nothing on the sink side will make this faster — look at the backend
(`progress_running_jobs`, `progress_block_rate`) or at the module itself.

**Database-bound.** `blocks_ahead` climbs steadily, `quota_used` rises toward 50% and the
`WARN` fires, `applier_busy` approaches 100%, `quota_wait` grows every tick. Raise
`--spool-max-size` to absorb bursts, tune `--db-write-target-duration`, check
`--write-mode`, or drop constraints for the backfill and apply them after.

**CPU-bound in the sink.** Stream wait is small, `applier_busy` is low, but
`Block Processing Duration` and the `Unmarshalling` / `Message Walk` lines are large and
`recent` is not improving. Look at `--decode-batch-size` and the decode workers.

**Spool-disk-bound.** `Spool Write Duration` dominates the ladder while `applier_busy` stays
low. The spool directory's disk is the limit — move `--spool-dir` to faster storage.

**Stalled.** `Last Block Process At` stops advancing and `data_msg_rate` falls to zero. The
stream is not delivering; `open_age` grows until `--spool-max-idle` seals what is open.

---

## 10. Related flags

| Flag | Effect on these numbers |
|---|---|
| `--spool-dir` | Where segments are written. Drives `Spool Write Duration`. |
| `--spool-max-size` | Disk budget; the denominator of `quota_used` and the threshold for the falling-behind `WARN`. Bounds how far ahead of the database the stream may run. |
| `--spool-max-idle` | Idle seal timer; drives `sealed_by_idle` and bounds `open_age`. |
| `--db-write-target-duration` | Target that `apply_duration` converges on; drives `segment_target`. |
| `--write-mode` | How a sealed segment reaches the database; changes the relationship between `total_applied_bytes` and `db_bytes`. |
| `--decode-batch-size` | How many blocks are decoded together; sizes the CPU stage behind `Unmarshalling` and `Message Walk`. |

All the spool flags are backfill-only — they are ignored once the stream reaches chain head.

See also the [Sink Config Reference](./sink-config.md) for how a sink is configured, and the
[Re-org Handling Reference](./reorg-handling.md) for what an undo does to the write path.
