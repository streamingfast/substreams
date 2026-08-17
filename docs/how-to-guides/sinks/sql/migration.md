# Migrating from substreams-sink-sql

The standalone `substreams-sink-sql` binary is now part of the `substreams` CLI as `substreams sink postgres` and `substreams sink clickhouse`. Existing databases keep working: the cursor tables and schemas are unchanged, so the new CLI resumes exactly where the standalone binary left off.

## Command mapping

The engine is now part of the command name and must match your DSN scheme. There is no `run` subcommand: the engine command itself runs the sink.

| substreams-sink-sql | substreams CLI |
| --- | --- |
| `run $DSN manifest.yaml 100:200` | `substreams sink postgres manifest.yaml -s 100 -t 200 --dsn $DSN` |
| `from-proto $DSN manifest.yaml` | `substreams sink postgres manifest.yaml --dsn $DSN` (mode is auto-detected) |
| `setup $DSN manifest.yaml` | `substreams sink postgres setup manifest.yaml --dsn $DSN` |
| `generate-csv $DSN manifest.yaml 0:100` | `substreams sink postgres generate-csv manifest.yaml -t 100 --dsn $DSN` |
| `inject-csv $DSN ./csv table 0:100` | `substreams sink postgres inject-csv ./csv table 0:100 --dsn $DSN` |
| `tools --dsn $DSN cursor read` | `substreams sink postgres tools cursor read --dsn $DSN` |
| `create-user ...` | removed |

For ClickHouse targets, replace `postgres` with `clickhouse` in every command. `generate-csv` and `inject-csv` are PostgreSQL-only.

There is no separate `from-proto` command anymore: the engine command (and `setup`) detects the mode from the output module's type. A module producing `sf.substreams.sink.database.v1.DatabaseChanges` runs in Database Changes Mode and uses your `schema.sql`; any other output type runs in Relational Mappings Mode, whose schema is derived from the protobuf definition.

## Flag changes

- The DSN is no longer a positional argument. Pass `--dsn`, or set the `SUBSTREAMS_SINK_DSN` environment variable. `${VAR}` expansion inside the DSN still works.

  ```bash
  # before
  substreams-sink-sql run "psql://user:pass@localhost:5432/db?sslmode=disable" manifest.yaml 100:200

  # after
  substreams sink postgres manifest.yaml -s 100 -t 200 --dsn "psql://user:pass@localhost:5432/db?sslmode=disable"

  # or keep the DSN out of the command line entirely
  export SUBSTREAMS_SINK_DSN="psql://user:pass@localhost:5432/db?sslmode=disable"
  substreams sink postgres manifest.yaml -s 100 -t 200
  ```
- The block range is no longer a positional argument. Use `-s/--start-block` and `-t/--stop-block`, like `substreams run`. `inject-csv` keeps its positional `<start>:<stop>` file range.
- ClickHouse flags lost their prefix and only exist on `substreams sink clickhouse`: `--cluster`, `--cursor-file-path`, `--sink-info-folder`, `--query-retry-count`, `--query-retry-sleep`.
- `--metrics-listen-addr` is replaced by the standard sink flag `--prometheus-addr` (same `localhost:9102` default).
- `--flush-interval` (deprecated alias) is gone; use `--batch-block-flush-interval`.
- The misspelled `--on-module-hash-mistmatch` alias is gone; use `--on-module-hash-mismatch`.

## Operators: Docker image and service arguments

- The Docker image changes from `ghcr.io/streamingfast/substreams-sink-sql` to `ghcr.io/streamingfast/substreams`. Both images use their binary as entrypoint, so only the container arguments change:
  - before: `run $DSN /data/manifest.yaml 100:200 --on-module-hash-mismatch=warn`
  - after: `sink postgres /data/manifest.yaml -s 100 -t 200 --dsn $DSN --on-module-hash-mismatch=warn`
- Instead of putting the DSN (and its password) in the arguments, you can set the `SUBSTREAMS_SINK_DSN` environment variable on the container.
- Prometheus scraping: `--metrics-listen-addr` is now `--prometheus-addr`, same `localhost:9102` default. Bind it to `0.0.0.0:9102` if you scrape from outside the container.
- pprof no longer listens by default; opt in with `--pprof-listen-addr`.

## Cursor compatibility

- Database Changes Mode stores its cursor in the same `cursors` table, keyed by module hash. Point the new CLI at the same database and it resumes from the stored cursor.
- Relational-mappings mode is also unchanged: `_cursor_` table on PostgreSQL, cursor file on ClickHouse (`--cursor-file-path`, previously `--clickhouse-cursor-file-path`, same `cursor.txt` default).

## Environment variables

`PG_DSN` / `CLICKHOUSE_DSN` were shell conventions from the old README, not read by the binary. The new CLI reads `SUBSTREAMS_SINK_DSN` when `--dsn` is not provided. Authentication is unchanged: `SUBSTREAMS_API_TOKEN`/`SUBSTREAMS_API_KEY` and `.substreams.env` work as with every other `substreams` command.
