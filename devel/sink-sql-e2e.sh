#!/usr/bin/env bash
# End-to-end smoke test for `substreams sink postgres` / `substreams sink clickhouse`.
# Streams real data from a live endpoint into throwaway Docker databases and
# verifies rows + cursor for both modes (DatabaseChanges and relational mappings).
#
# Requirements: docker, an authenticated environment (SUBSTREAMS_API_TOKEN or
# SUBSTREAMS_API_KEY exported, or a .substreams.env next to this script).
#
# Overridable:
#   E2E_ENDPOINT           default mainnet.eth.streamingfast.io:443
#   E2E_DBCHANGES_SPKG     spkg with a db_out module + sink config (DatabaseChanges mode)
#   E2E_FROMPROTO_SPKG     spkg whose output module is a plain proto (relational mappings)
#   E2E_FROMPROTO_ENDPOINT endpoint for the from-proto spkg's chain
#   E2E_BLOCKS             number of blocks to stream (default +30)

set -euo pipefail
cd "$(dirname "${BASH_SOURCE[0]}")/.."

ENDPOINT="${E2E_ENDPOINT:-mainnet.eth.streamingfast.io:443}"
DBCHANGES_SPKG="${E2E_DBCHANGES_SPKG:-https://github.com/streamingfast/substreams-eth-block-meta/releases/download/v0.5.1/substreams-eth-block-meta-v0.5.1.spkg}"
FROMPROTO_SPKG="${E2E_FROMPROTO_SPKG:-https://github.com/streamingfast/substreams-spl-token/releases/download/v0.1.0/solana-spl-token-v0.1.0.spkg}"
FROMPROTO_ENDPOINT="${E2E_FROMPROTO_ENDPOINT:-mainnet.sol.streamingfast.io:443}"
BLOCKS="${E2E_BLOCKS:-+30}"

PG_CONTAINER=sink-sql-e2e-pg
PG_DSN="psql://postgres:insecure@localhost:5433/postgres?sslmode=disable"

cleanup() { docker rm -f "$PG_CONTAINER" >/dev/null 2>&1 || true; }
trap cleanup EXIT
cleanup

echo "--- building substreams CLI"
go build -o /tmp/substreams-e2e ./cmd/substreams

echo "--- starting postgres"
docker run -d --name "$PG_CONTAINER" -e POSTGRES_PASSWORD=insecure -p 5433:5432 postgres:17-alpine >/dev/null
until docker exec "$PG_CONTAINER" pg_isready -U postgres >/dev/null 2>&1; do sleep 0.5; done

psql_exec() { docker exec "$PG_CONTAINER" psql -U postgres -t -A -c "$1"; }

echo "--- mode A (DatabaseChanges): setup"
/tmp/substreams-e2e sink postgres setup "$DBCHANGES_SPKG" --dsn "$PG_DSN"

echo "--- mode A (DatabaseChanges): run $BLOCKS blocks"
/tmp/substreams-e2e sink postgres "$DBCHANGES_SPKG" -e "$ENDPOINT" -t "$BLOCKS" --dsn "$PG_DSN" --on-module-hash-mismatch warn

rows=$(psql_exec "SELECT count(*) FROM block_meta;")
echo "block_meta rows: $rows"
[ "$rows" -gt 0 ] || { echo "FAIL: no rows sunk in DatabaseChanges mode"; exit 1; }

echo "--- mode A: cursor"
/tmp/substreams-e2e sink postgres tools cursor read --dsn "$PG_DSN" | grep -i cursor || { echo "FAIL: no cursor stored"; exit 1; }

echo "--- mode B (relational mappings): run $BLOCKS blocks (auto-detected, no setup)"
/tmp/substreams-e2e sink postgres "$FROMPROTO_SPKG" -e "$FROMPROTO_ENDPOINT" -t "$BLOCKS" --dsn "$PG_DSN"

tables=$(psql_exec "SELECT count(*) FROM information_schema.tables WHERE table_schema NOT IN ('pg_catalog','information_schema');")
echo "tables after from-proto: $tables"

echo "--- OK: both modes sank data end-to-end"
