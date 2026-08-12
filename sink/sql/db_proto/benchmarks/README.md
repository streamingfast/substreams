# from-proto Postgres sink: ingestion benchmarks

Two suites, answering the two questions that decide the design in
`plans/2026-08-07-fast-local-buffer-copy.md`:

- **`TestCopyVsInsert`** — how fast can PostgreSQL absorb rows, per strategy? Runs against
  a real server in a testcontainer. Every artifact is materialised on disk *before* any
  timer starts, so a measured duration is transport plus server work, never row
  generation — unless the variant's real implementation would do that work at flush time
  too.
- **`TestClientEncodeCeiling`** — how fast can the sink *produce* rows, on one core, with
  no database at all? Runs the production path: `proto.Unmarshal` into `dynamicpb`, then
  `WalkMessageDescriptorAndInsertWithDialect` with the real Postgres dialect. Only the
  `Inserter` changes between variants. No docker needed.

The second one turned out to matter more than the first. See *What the numbers say*.

## Running

The correctness tests always run; only the measurements are gated, so plain
`go test ./...` stays fast. It does need a container runtime:

| gate | covers | |
|---|---|---|
| *(none)* | `TestPgCopyBinaryRoundTrip` | correctness; always runs, and needs a container runtime |
| `SF_SINK_SQL_BENCHMARKS=true` | everything else | measurements; they assert nothing and cost minutes |

`TestCopyVsInsert` still needs `SF_SINK_SQL_BENCHMARKS=true`; its container now comes on
its own.

```bash
# correctness first: the binary encoder must be byte-identical to a parameterised INSERT
go test ./sink/sql/db_proto/benchmarks/ -run TestPgCopyBinaryRoundTrip -v

# server side
PGBENCH_ROWS=250000 PGBENCH_REPEAT=2 \
  go test ./sink/sql/db_proto/benchmarks/ -run TestCopyVsInsert -v -timeout 30m

# client side, no docker
go test ./sink/sql/db_proto/benchmarks/ -run TestClientEncodeCeiling -v
```

| Variable | Default | Meaning |
|---|---|---|
| `PGBENCH_ROWS` | `250000` | rows in the dataset |
| `PGBENCH_REPEAT` | `1` | passes over the variant set; best duration is reported |
| `PGBENCH_WITH_INDEX` | unset | add btrees on `id` and `_block_number_` before loading |
| `PGBENCH_PG_IMAGE` | `postgres:17-alpine` | server image |
| `PGBENCH_KEEP_DATA_DIR` | unset | reuse artifacts across runs instead of a temp dir |


## Variants

| Variant | What it stands for |
|---|---|
| `insert-1row-prepared` | `RowInserter` — one prepared `INSERT` per row, in one tx |
| `insert-multirow-built-at-flush` | `AccumulatorInserter` — build the giant `VALUES` text statement at flush, exec it. **The current baseline.** |
| `insert-multirow-built-at-flush-libpq` | same through `database/sql` + `lib/pq`, to size the driver's share |
| `insert-multirow-prebuilt-from-disk` | the same statements, already built on disk — isolates pure server-side cost |
| `copy-csv-from-disk` | `COPY FROM STDIN (FORMAT CSV)`, `io.Copy` from file |
| `copy-binary-from-disk` | `COPY FROM STDIN (FORMAT BINARY)`, `io.Copy` from file — **the proposed spill path** |
| `copy-binary-encoded-at-flush` | `pgx.CopyFrom` over in-memory rows — binary COPY without pre-encoding |

Correctness is not optional here: after every variant the table is fingerprinted in SQL
and compared against the same fingerprint computed in Go. A variant that loads different
data fails, however fast it was.

## Measured

250,000 rows, 13 columns (`BIGINT`, `TIMESTAMP`, `VARCHAR`, `BYTEA`, `INTEGER`,
`NUMERIC`, `BOOLEAN`, `DOUBLE PRECISION`, `TEXT[]`, `JSONB`), ~88 MiB of data.
`postgres:17-alpine` under OrbStack on an M-series laptop, best of 2 passes. Ratios,
not absolutes, are the point.

### No indexes (the default, the backfill case)

| variant | duration | rows/s | MiB/s | vs current |
|---|--:|--:|--:|--:|
| insert-1row-prepared | 22.199s | 11k | 4.1 | 0.10x |
| insert-multirow-built-at-flush | 2.281s | 110k | 40.1 | 1.00x |
| insert-multirow-built-at-flush-libpq | 2.296s | 109k | 39.8 | 0.99x |
| insert-multirow-prebuilt-from-disk | 1.989s | 126k | 46.0 | 1.15x |
| copy-csv-from-disk | 724ms | 345k | 123.1 | 3.15x |
| **copy-binary-from-disk** | **313ms** | **799k** | **280.3** | **7.29x** |
| copy-binary-encoded-at-flush | 339ms | 737k | 258.8 | 6.73x |

### With two secondary indexes

| variant | duration | vs current |
|---|--:|--:|
| insert-multirow-built-at-flush | 2.812s | 1.00x |
| copy-csv-from-disk | 1.135s | 2.48x |
| **copy-binary-from-disk** | **733ms** | **3.84x** |

## What constraints cost, and what COPY is still worth under them (`TestConstraintCost`)

500,000 rows through a relational shape — `blocks <- parents <- children`, children also
carrying a unique column — loaded by binary COPY and by multi-row INSERT, under each set
of constraints. `postgres:17-alpine`, M-series laptop.

| load runs with | COPY | vs bare | INSERT | vs bare | COPY gain |
|---|--:|--:|--:|--:|--:|
| no constraints | 127ms | 1.00x | 610ms | 1.00x | **4.79x** |
| primary keys | 552ms | 0.23x | 1.079s | 0.56x | 1.96x |
| primary keys + unique | 748ms | 0.17x | 1.255s | 0.49x | 1.68x |
| primary keys + unique + foreign keys | 4.063s | 0.03x | 4.639s | 0.13x | **1.14x** |
| loaded bare, constraints created after | 453ms | 0.28x | 940ms | 0.65x | 2.07x |

Single run, and the foreign-key variant is the noisiest of them: a second sample on the
same machine gave 163ms / 609ms / 847ms / 5.151s / 462ms, so read these as ratios with
about 15% of slack, more like 25% on the foreign-key row. The conclusions below survive
either sample.

**Foreign keys dominate everything else.** Primary keys cost 4.3x, uniques add little on
top, and foreign keys take the load from 748ms to 4.06s — a per-row referential check that
no write path avoids.

**Under full constraints the write path stops mattering**: 4.06s against 4.64s, a 1.14x
edge. All the work is server-side by then, so the buffer and binary COPY buy nothing. The
COPY advantage is 4.79x precisely when the load is bare, which is the whole argument for
loading bare and creating the constraints afterwards — 453ms for the same end state, 9x
cheaper than the 4.06s of loading with them in place.

## End to end against a real substreams (`live-benchmark.sh`)

The Go suites below isolate one stage each. `live-benchmark.sh` measures what an operator
actually experiences: stream, decode and load together, against a live endpoint. It runs
on Linux or macOS and needs only docker, python3, and either go or a prebuilt binary.

```bash
export SUBSTREAMS_API_KEY=...

cd sink/sql/db_proto/benchmarks
./live-benchmark.sh                                    # 10k and 50k blocks
SIZES="10000 50000 200000" ./live-benchmark.sh
ENDPOINT=https://mainnet.eth.ca.streamingfast.io ./live-benchmark.sh
WARM=1 ./live-benchmark.sh                             # range nobody has streamed yet
```

It builds the binary from the checkout, starts its own PostgreSQL container, runs both
variants at each size and prints the table. Everything lands in
`./.sinkbench` (gitignored); results accumulate in `results.tsv` across invocations, so
several endpoints can be compared in one table.

### Requirements

On a fresh Ubuntu (24.04 tested), the script itself needs only:

```bash
sudo apt-get update
sudo apt-get install -y docker.io python3 ca-certificates
sudo usermod -aG docker "$USER"   # then log out and back in, or: newgrp docker
```

`python3` and `ca-certificates` ship with a normal Ubuntu install but are absent from
minimal and container images; without the certificates every HTTPS call fails with
`x509: certificate signed by unknown authority`.

Then the binary, either of two ways:

**Bring a prebuilt binary** — nothing else to install, no repo checkout, no Go:

```bash
# on any machine with Go:
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o substreams ./cmd/substreams
scp substreams live-benchmark.sh live-report.py ubuntu-host:~/bench/

# on the Ubuntu host:
cd ~/bench && SUBSTREAMS_BIN=./substreams ./live-benchmark.sh
```

The build is CGO-free, so it cross-compiles cleanly and runs on a bare Ubuntu.

**Or build on the machine** — needs `git`, a checkout, and **Go 1.26**, which apt does
not carry (24.04 ships 1.22):

```bash
sudo snap install go --classic          # or install the official tarball
sudo apt-get install -y git
git clone <repo> && cd substreams/sink/sql/db_proto/benchmarks
./live-benchmark.sh
```

Also needed: `SUBSTREAMS_API_KEY`, outbound HTTPS to the endpoint, to `spkg.io` for the
package and to Docker Hub for the PostgreSQL image, and disk for the database.

**Disk is the one that bites.** At roughly 110 KB per block that is about 5.5 GB for the
default sizes and about 20 GB at 200,000 blocks, plus the `postgres:17-alpine` image
(291 MB) and the local buffer directory, which `BUFFER_MAX` bounds. Note the space has to
be free where *docker* stores its data, which is often a different filesystem from the
working directory. The script prints both before it starts and warns when the estimate
does not fit.

Running out mid-run is worth recognising, because the sink only reports
`driver: bad connection` -- PostgreSQL dies part-way through a write and the client just
sees the connection go away. On any such failure the script now dumps the database's own
log, which says `No space left on device` plainly.

| variable | default | |
|---|---|---|
| `ENDPOINT` | `https://mainnet.eth.streamingfast.io` | |
| `SIZES` | `10000 50000` | block counts to measure |
| `START_BLOCK` | `20000000` | |
| `PACKAGE` / `MODULE` / `TABLE` | `erc20-balance-changes` / `map_balance_changes` / `balancechange` | |
| `WARM` | `0` | stream the range once first, see below |
| `SUBSTREAMS_BIN` | *(built from source)* | skip the build |
| `PG_PORT` / `PG_IMAGE` / `PG_CONTAINER` | `55432` / `postgres:17-alpine` / `sinkbench-pg` | |
| `BUFFER_MAX` | `8GiB` | `--local-buffer-max-size` |
| `BLOCK_BATCH` | *(sink default, 25)* | `--block-batch-size`; lower it if a backend is killed for memory |

**Warming, `WARM=1`, is off by default.** The server-side cache holds for about 30 days,
so a range that has been streamed recently is already warm and warming it again only
costs time.

It matters on a range nobody has touched. A cold range is dominated by that first pass
rather than by the sink, so whichever variant ran first would pay for the other's stream
as well as its own: cold, a 5,000-block comparison read 9.4x where warm it is 2.9x.
Endpoints also cap how many *uncached* blocks a single request may process —
`mainnet.eth.ca` rejects anything over 10,000 outright — so a cold large range fails
rather than merely running slowly. The script recognises that error and points at
`WARM=1`.

**These are totals, to durable.** The clock is launch to exit, and the sink only exits
after `HandleBlockRangeCompletion` has flushed what is held and drained the buffer. Every
check on the resulting data runs strictly after the process is gone, so a variant that
deferred work past exit reports fewer rows rather than a better time. The `drain` column
is the part of the run that happened after the stream finished — a path that merely
deferred its work would show a long tail there.

### Measured

`erc20-balance-changes@v1.4.0` / `map_balance_changes`, Ethereum mainnet from block
20,000,000, into `postgres:17-alpine`. Every row verified identical between variants:
same rows, same blocks, same fingerprint.

**On a server** — AWS `c5.2xlarge` in `us-east-2`, EBS `gp3` at 1000 MB/s and 10,000 IOPS:

| endpoint | blocks | rows | accumulator | buffer | speedup | acc rows/s | buffer rows/s |
|---|--:|--:|--:|--:|--:|--:|--:|
| mainnet.eth | 10,000 | 3,375,211 | 94.9s | 18.3s | 5.19x | 35,566 | 184,438 |
| mainnet.eth | 50,000 | 17,018,743 | 473.8s | 66.6s | 7.11x | 35,919 | 255,537 |
| mainnet.eth.ca | 10,000 | 3,375,211 | 92.4s | 16.5s | 5.60x | 36,528 | 204,558 |
| mainnet.eth.ca | 50,000 | 17,018,743 | 445.9s | 65.7s | 6.79x | 38,167 | 259,037 |
| mainnet.eth.ca | 200,000 | 66,273,320 | 1741.0s | 304.8s | 5.71x | 38,066 | 217,432 |

Reproducibility on this machine is good: 10,000 blocks on mainnet.eth.ca measured twice
came out 90.6s/16.3s and 92.4s/16.5s, a 2.0% and 1.2% spread. The two endpoints agree to
within a few percent as well.

**The speedup does not trend with size.** It is 5.6x at 10,000, 6.8x at 50,000 and 5.7x
at 200,000 — a hump, not a slope, and the same shape appears on both endpoints. The two
sides explain it separately:

- The **accumulator is almost perfectly linear**: 35.6k, 35.9k, 36.5k, 38.2k, 38.1k rows/s
  across every size and endpoint. It is bound by one core building a multi-megabyte
  statement and one backend parsing it, and neither cares how large the table has grown.
- The **buffer is not**: 184k to 259k rows/s. Fixed startup — schema setup, connection,
  the segment sizer ramping from its 8 MiB floor — is amortised over less data at 10,000,
  and by 200,000 the table is around 20 GB and PostgreSQL's own ingest has slowed a
  little. The best case sits in between.

So the honest headline for server hardware is a range, 5.6x to 7.1x, rather than a trend.

**On a laptop** — M-series, `postgres:17-alpine` in Docker Desktop:

| endpoint | blocks | rows | accumulator | buffer | speedup |
|---|--:|--:|--:|--:|--:|
| mainnet.eth | 10,000 | 3,375,211 | 40.0s | 12.2s | 3.28x |
| mainnet.eth | 50,000 | 17,018,743 | 220.7s | 70.2s | 3.14x |
| mainnet.eth | 200,000 | 66,273,320 | 749.6s | 270.2s | 2.77x |
| mainnet.eth.ca | 10,000 | 3,375,211 | 72.9s | 55.2s | 1.32x |
| mainnet.eth.ca | 50,000 | 17,018,743 | 353.8s | 311.3s | 1.14x |
| mainnet.eth.ca | 200,000 | 66,273,320 | 1297.8s | 939.9s | 1.38x |

Repeat measurement, laptop mainnet.eth.ca 200,000 accumulator: 1297.8s and 1364.8s,
5.2% spread.

### What the two machines say

**The server is where the accumulator hurts most.** The buffer lands near the laptop's
figures (18.3s against 12.2s at 10,000) while the accumulator is more than twice as slow
(94.9s against 40.0s). That is the shape to expect: the accumulator is bound by one core
building and one backend parsing a multi-megabyte statement, and the c5's single-core
throughput is well below an M-series laptop's, whereas the buffer spends its time in COPY
and in the stream. Laptop figures are the conservative case, not the flattering one.

The laptop's mainnet.eth column shows the ratio falling with size (3.28x, 3.14x, 2.77x)
where the server shows a hump. Neither is a law; both are the same two curves — a flat
accumulator and a buffer whose throughput depends on startup amortisation and on how large
the table has grown — sampled on different hardware.

**The laptop's `.ca` rows are a network artifact, not an endpoint property.** They were
taken over wifi to a geographically distant endpoint, and the same endpoint from a server
behaves like any other. What they do still illustrate is the regime rather than the
cause: when blocks arrive slowly for *any* reason, the sink is stream-bound, there is
little sink cost left to remove, and the ratio collapses toward 1x — 1.1x to 1.4x here,
at every size including 10,000. Measuring the sink over a slow or distant link measures
the link.

**Throughput is flat in data volume.** On the laptop against mainnet.eth the buffer holds
276k, 242k and 245k rows/s across 10k, 50k and 200k, and the accumulator 84k, 77k and
88k. Neither degrades from 3.4M to 66M rows.

## Parallel decode scaling (`TestClientDecodeScaling`)

The per-block work the sinker's decoder parallelises — unmarshal, walk, buffer the
inserts — on a wide entity, 16 cores available:

| workers | rows/s | speedup |
|--:|--:|--:|
| 1 | 129k | 1.00x |
| 2 | 220k | 1.70x |
| 4 | 355k | 2.75x |
| 8 | 534k | 4.13x |
| 15 | 548k | 4.24x |

Flat past eight: the work is allocator-bound before it runs out of cores. That is why
the decoder defaults to one worker per core less one, capped at eight.

## What the numbers say

**For wide entities the sink's own CPU is the bottleneck, not PostgreSQL.** One core
produces 96k rows/s; binary COPY absorbs 799k. PostgreSQL has ~8x of headroom the current
single-threaded handler cannot reach. For narrow entities the two are within 10%. The
biggest available lever is therefore **parallelising decode + walk + encode across
cores** — blocks are independent — not anything on the database side.

**Binary COPY is worth ~7x over the current multi-row INSERT**, and 2.3x over text/CSV
COPY. It is also the only strategy that can absorb a parallelised client.

**Pre-encoding to disk buys only ~8%** (313ms vs 339ms). So spilling PGCOPY bytes to disk
does not justify itself on throughput. It justifies itself by moving encoding off the
flush path so the stream keeps being consumed during a COPY, and by making downloaded
blocks survive a restart. A spill design that still blocks the stream would be strictly
worse than `pgx.CopyFrom` over in-memory rows.

**Building the SQL string is not the bottleneck.** Prebuilt-from-disk is only 1.15x
better than building at flush time, so `ValueToString` plus `strings.Builder` are ~13% of
the current cost. The other ~87% is the server parsing a multi-megabyte statement.

**The driver is irrelevant.** `lib/pq` and pgx in simple-protocol mode are within noise
(2.296s vs 2.281s). No reason to switch drivers for the INSERT path.

**Per-row prepared INSERT is 10x worse than the default.** It is now only the fallback
for a schema whose foreign keys form a cycle and therefore cannot be ordered; constraints
alone no longer select it, since the tables are ordered by their foreign keys instead.

**Indexes eat over half the COPY advantage** — 7.29x drops to 3.84x with two btrees.
COPY removes client and parse cost, not index maintenance or WAL amplification.

### Hypotheses tested and rejected

- **NUMERIC encoding via `big.Int` is the hot spot.** Mapping every unsigned integer to
  `BIGINT` instead of `NUMERIC` moves wide-entity throughput from 96k to 100k rows/s —
  4%. Not worth a schema change or a hand-rolled numeric encoder. (`uint32` → `NUMERIC`
  at `sql/postgres/types.go:75` is still arguably wrong, since `uint32` fits `BIGINT`
  exactly, but not for performance reasons.)
- **Text encoding is slower than binary encoding client-side.** It is the opposite: text
  literals are ~1 µs/row *cheaper* to produce than binary COPY tuples. Binary wins on the
  round trip because the server side is 7x faster, not because the client side is.

One optimisation was found and kept: caching the pgtype encode plan per column in
`pgcopy.Writer` instead of letting `Map.Encode` resolve one per value. +8% on wide
entities, +3% on narrow, no downside.

## Findings

`postgres.ValueToString` renders raw bytes as `E'<hex>'::BYTEA`, which casts the hex
*text* to bytea — 32 binary bytes are stored as the 64 ASCII characters of their hex
representation. `TestValueToStringRawBytesAreDoubleEncoded` pins the current behaviour
against a live server; the correct literal is `'\x<hex>'::bytea`. This affects the
`db_proto` accumulator path with `--bytes-encoding=raw` today, independently of any of
the work above.
