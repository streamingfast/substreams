#!/usr/bin/env bash
#
# End-to-end comparison of the from-proto PostgreSQL sink's two ingestion paths, against
# a live Substreams endpoint.
#
#   accumulator  the current path: rows are buffered in memory as SQL literals and
#                flushed as one large multi-row INSERT per table, synchronously
#   buffer        --local-buffer: rows are written to disk in the binary COPY wire format
#                and loaded by a background goroutine, one transaction per segment
#
# Both variants run the same binary over the same block range; only the flag differs.
# The wall clock is launch to exit, and every check on the resulting data runs strictly
# after the process has exited, so a variant that deferred work past exit would show up
# short rather than fast.
#
# Usage:
#   export SUBSTREAMS_API_KEY=...
#   ./live-benchmark.sh
#   SIZES="10000 50000 200000" ./live-benchmark.sh
#   ENDPOINT=https://mainnet.eth.ca.streamingfast.io ./live-benchmark.sh
#   WARM=1 ./live-benchmark.sh          # first run over a range nobody has streamed
#
# Requirements: docker, python3, and either go or a prebuilt binary in SUBSTREAMS_BIN.

set -uo pipefail

ENDPOINT=${ENDPOINT:-https://mainnet.eth.streamingfast.io}
PACKAGE=${PACKAGE:-erc20-balance-changes}
MODULE=${MODULE:-map_balance_changes}
TABLE=${TABLE:-balancechange}
START_BLOCK=${START_BLOCK:-20000000}
SIZES=${SIZES:-"10000 50000"}
WARM=${WARM:-0}
WARM_CHUNK=${WARM_CHUNK:-10000}
BUFFER_MAX=${BUFFER_MAX:-8GiB}
BLOCK_BATCH=${BLOCK_BATCH:-}
# Rough on-disk cost of one block for the default package, measured at ~324 bytes per row
# and ~338 rows per block. Only used to size the preflight check.
BYTES_PER_BLOCK=${BYTES_PER_BLOCK:-110000}

PG_CONTAINER=${PG_CONTAINER:-sinkbench-pg}
PG_PORT=${PG_PORT:-55432}
PG_IMAGE=${PG_IMAGE:-postgres:17-alpine}
WORKDIR=${WORKDIR:-./.sinkbench}
RESULTS="$WORKDIR/results.tsv"

log() { printf '\n=== %s ===\n' "$*"; }
die() { printf 'error: %s\n' "$*" >&2; exit 1; }

human_bytes() { python3 -c "
n = float($1)
for unit in ('B', 'KiB', 'MiB', 'GiB', 'TiB'):
    if n < 1024:
        print('%.1f%s' % (n, unit)); break
    n /= 1024
"; }

docker_root() { docker info --format '{{.DockerRootDir}}' 2>/dev/null || echo /var/lib/docker; }

# free_space_bytes <path> -- bytes available on the filesystem holding path.
#
# Walks up until it finds something that exists: the docker root is often a path inside a
# VM that the host cannot see, in which case there is nothing useful to report.
free_space_bytes() {
  local path=$1
  while [ -n "$path" ] && [ "$path" != "/" ] && [ ! -d "$path" ]; do path=$(dirname "$path"); done
  [ -d "$path" ] || return 0
  df -Pk "$path" 2>/dev/null | awk 'NR==2 {print $4*1024}'
}

free_space() {
  local bytes
  bytes=$(free_space_bytes "$1")
  if [ -z "$bytes" ]; then echo "unknown"; else human_bytes "$bytes"; fi
}

# --- prerequisites -----------------------------------------------------------------

command -v docker  >/dev/null || die "docker is required"
command -v python3 >/dev/null || die "python3 is required"
[ -n "${SUBSTREAMS_API_KEY:-}" ] || [ -n "${SUBSTREAMS_API_TOKEN:-}" ] ||
  die "set SUBSTREAMS_API_KEY (or SUBSTREAMS_API_TOKEN)"

mkdir -p "$WORKDIR"

BIN=${SUBSTREAMS_BIN:-}
if [ -z "$BIN" ]; then
  command -v go >/dev/null || die "set SUBSTREAMS_BIN or install go to build from source"
  BIN="$(cd "$WORKDIR" && pwd)/substreams"
  root=$(git rev-parse --show-toplevel 2>/dev/null) || die "run inside the repo, or set SUBSTREAMS_BIN"
  log "building $BIN"
  ( cd "$root" && go build -o "$BIN" ./cmd/substreams ) || die "build failed"
fi
[ -x "$BIN" ] || die "$BIN is not executable"

# --- database ----------------------------------------------------------------------

if ! docker ps --format '{{.Names}}' | grep -qx "$PG_CONTAINER"; then
  log "starting $PG_CONTAINER ($PG_IMAGE) on port $PG_PORT"
  docker rm -f "$PG_CONTAINER" >/dev/null 2>&1
  docker run -d --name "$PG_CONTAINER" \
    -e POSTGRES_PASSWORD=bench -e POSTGRES_USER=bench -e POSTGRES_DB=bench \
    -p "${PG_PORT}:5432" "$PG_IMAGE" >/dev/null || die "could not start postgres"

  # Docker can take a moment to accept exec into a freshly started container, and the
  # first boot also runs initdb, so give it room rather than racing it.
  ready=0
  for attempt in $(seq 1 120); do
    if docker exec "$PG_CONTAINER" pg_isready -U bench >/dev/null 2>&1; then
      ready=1
      break
    fi
    [ $(( attempt % 15 )) -eq 0 ] && printf '  still waiting for postgres (%ss)\n' "$attempt"
    sleep 1
  done

  if [ "$ready" -ne 1 ]; then
    printf 'container status: %s\n' "$(docker ps -a --filter "name=${PG_CONTAINER}" --format '{{.Status}}')" >&2
    docker logs --tail 20 "$PG_CONTAINER" >&2 2>&1
    die "postgres never became ready"
  fi
fi

psql_q() { docker exec -i "$PG_CONTAINER" psql -U bench -d bench -t -A -c "$1"; }

DSN_BASE="psql://bench:bench@localhost:${PG_PORT}/bench?sslmode=disable"

# --- warming -----------------------------------------------------------------------
#
# Off by default: the server-side cache stays valid for about 30 days, so a range that
# has been streamed recently is already warm and warming it again just costs time.
#
# It matters on a range nobody has touched. A cold range is dominated by the first pass
# rather than by the sink, so whichever variant ran first would pay for the other's
# stream as well as its own: cold, a 5,000-block comparison reads 9.4x where warm it is
# 2.9x. Set WARM=1 for a range you have not streamed before.

warm() {
  local total=$1 off s
  log "warming ${total} blocks from ${START_BLOCK} in chunks of ${WARM_CHUNK}"
  for (( off=0; off<total; off+=WARM_CHUNK )); do
    s=$(( START_BLOCK + off ))
    "$BIN" run "$PACKAGE" "$MODULE" -e "$ENDPOINT" --production-mode \
      -s "$s" -t "+${WARM_CHUNK}" -o clock > "$WORKDIR/warm_${s}.log" 2>&1
    if ! grep -q "Completed successfully" "$WORKDIR/warm_${s}.log"; then
      printf '  chunk %s FAILED, see %s\n' "$s" "$WORKDIR/warm_${s}.log"
      tail -3 "$WORKDIR/warm_${s}.log" | sed 's/^/    /'
      return 1
    fi
    printf '  warmed %s..+%s\n' "$s" "$WARM_CHUNK"
  done
}

# --- one measured run --------------------------------------------------------------

measure() { # measure <size> <variant>
  local size=$1 variant=$2
  local schema="sb_${variant}_${size}"
  local bufferdir="$WORKDIR/buffer_${variant}_${size}"
  local log="$WORKDIR/log_${variant}_${size}.txt"

  psql_q "DROP SCHEMA IF EXISTS ${schema} CASCADE;" >/dev/null 2>&1
  rm -rf "$bufferdir"

  local extra=()
  [ "$variant" = buffer ] && extra=(--local-buffer "$bufferdir" --local-buffer-max-size "$BUFFER_MAX")
  [ -n "$BLOCK_BATCH" ] && extra+=(--block-batch-size "$BLOCK_BATCH")

  local t0 t1 rc
  t0=$(python3 -c 'import time;print(time.monotonic())')

  "$BIN" sink postgres "$PACKAGE" "$MODULE" \
    --dsn "${DSN_BASE}&schemaName=${schema}" -e "$ENDPOINT" --no-constraints \
    -s "$START_BLOCK" -t "+${size}" "${extra[@]}" > "$log" 2>&1
  rc=$?

  t1=$(python3 -c 'import time;print(time.monotonic())')

  if [ "$rc" -ne 0 ]; then
    printf '  %s FAILED (rc=%s), see %s\n' "$variant" "$rc" "$log"
    tail -3 "$log" | sed 's/^/    /'

    # A range nobody has streamed before trips the endpoint's cap on uncached blocks.
    if grep -q "limit-processed-blocks" "$log"; then
      printf '  this range is cold: re-run with WARM=1 to stream it once first\n'
    fi

    # "bad connection" means the server went away mid-statement rather than refusing the
    # work, so the reason is on the PostgreSQL side and not in this log.
    if grep -qE "bad connection|connection reset|EOF" "$log"; then
      printf '\n  the database dropped the connection, so the reason is in its log:\n'
      docker logs --tail 25 "$PG_CONTAINER" 2>&1 | sed 's/^/    /'

      local pglog
      pglog=$(docker logs --tail 300 "$PG_CONTAINER" 2>&1)

      if grep -qiE "no space left on device|could not write to file|could not extend file" <<< "$pglog"; then
        printf '\n  the database ran out of disk. this benchmark writes about %s per\n' "$(human_bytes "$BYTES_PER_BLOCK")"
        printf '  block, so %s blocks needs roughly %s, plus the local buffer directory.\n' \
               "$size" "$(human_bytes $(( size * BYTES_PER_BLOCK )))"
        printf '  free space now: docker %s, workdir %s\n' \
               "$(free_space "$(docker_root)")" "$(free_space "$WORKDIR")"
      elif grep -qiE "out of memory|terminated by signal 9" <<< "$pglog"; then
        printf '\n  a backend was killed for memory. the accumulator sends one INSERT per\n'
        printf '  batch, so its statement grows with --block-batch-size (default 25).\n'
        printf '  retry with a smaller batch: BLOCK_BATCH=5 %s\n' "$0"
      fi
    fi
  fi

  # Strictly after the process exits. A variant still flushing in the background would
  # report fewer rows here, not a better time.
  local stats
  stats=$(psql_q "SELECT count(*) || '|' || (SELECT count(*) FROM ${schema}._blocks_) || '|' ||
                         sum(_block_number_)::text
                  FROM ${schema}.${TABLE};" 2>/dev/null)

  python3 - "$ENDPOINT" "$size" "$variant" "$t0" "$t1" "$stats" "$rc" "$RESULTS" \
           "$log" <<'PYEOF'
import datetime
import json
import sys

endpoint, size, variant = sys.argv[1:4]
t0, t1 = float(sys.argv[4]), float(sys.argv[5])
stats, rc, results, logpath = sys.argv[6], sys.argv[7], sys.argv[8], sys.argv[9]

rows, blocks, fp = (stats.split('|') + ['', '', ''])[:3]
seconds = t1 - t0


def timestamp(line):
    """The sink logs as JSON when stderr is not a terminal and as console text when it
    is, and which one you get varies by platform. Handle both rather than silently
    losing the drain figure to a format difference."""
    line = line.strip()
    if line.startswith('{'):
        try:
            return datetime.datetime.fromisoformat(json.loads(line)['timestamp'])
        except (ValueError, KeyError, json.JSONDecodeError):
            return None
    try:
        return datetime.datetime.fromisoformat(line.split(' ', 1)[0])
    except (ValueError, IndexError):
        return None


# How much of the run happened after the stream was already done, i.e. flushing what was
# still buffered. A path that merely deferred its work would show a long tail here rather
# than a genuinely shorter run.
drain = ''
try:
    stream_end = run_end = None
    with open(logpath, errors='replace') as fh:
        for line in fh:
            when = timestamp(line)
            if when is None:
                continue
            run_end = when
            if 'reached your stop block' in line and stream_end is None:
                stream_end = when
    if stream_end and run_end:
        drain = '%.1f' % (run_end - stream_end).total_seconds()
except OSError:
    pass

with open(results, 'a') as fh:
    fh.write('\t'.join([endpoint, size, variant, '%.1f' % seconds,
                        rows, blocks, fp, rc, drain]) + '\n')

tail = ('  drain=%ss' % drain) if drain else ''
print('  %-11s %8.1fs  rows=%s  rc=%s%s' % (variant, seconds, rows or '?', rc, tail), flush=True)
PYEOF

  psql_q "DROP SCHEMA IF EXISTS ${schema} CASCADE;" >/dev/null 2>&1
  rm -rf "$bufferdir"
}

# --- run ---------------------------------------------------------------------------

[ -f "$RESULTS" ] ||
  printf 'endpoint\tsize\tvariant\tseconds\trows\tblocks\tfingerprint\trc\tdrain\n' > "$RESULTS"

log "endpoint $ENDPOINT"
printf 'package %s / %s, from block %s, sizes: %s\n' "$PACKAGE" "$MODULE" "$START_BLOCK" "$SIZES"

largest=0
for size in $SIZES; do [ "$size" -gt "$largest" ] && largest=$size; done

needed=$(( largest * BYTES_PER_BLOCK ))
dockerfree=$(free_space_bytes "$(docker_root)")
printf 'disk: about %s needed for %s blocks; docker has %s free, workdir %s\n' \
  "$(human_bytes "$needed")" "$largest" "$(free_space "$(docker_root)")" "$(free_space "$WORKDIR")"
if [ -n "$dockerfree" ] && [ "$dockerfree" -lt "$needed" ]; then
  printf '\nwarning: the database will very likely run out of disk. PostgreSQL dies mid-write\n'
  printf '         when that happens and the sink reports only "driver: bad connection".\n'
  printf '         free some space, point WORKDIR elsewhere, or use a smaller SIZES.\n\n'
fi

if [ "$WARM" = 1 ]; then
  warm "$largest" || die "warming failed; a measured run over cold data would be dominated by it"
else
  printf 'warming skipped (WARM=1 to enable); the server-side cache holds for ~30 days\n'
fi

for size in $SIZES; do
  log "${size} blocks"
  measure "$size" accumulator
  measure "$size" buffer
done

log "results"
python3 "$(dirname "$0")/live-report.py" "$RESULTS"
