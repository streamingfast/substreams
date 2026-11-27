# CDC outputs

The CDC sink delivers change events (see the event envelope in the CDC guide) to the destination given by an output URL. Every output delivers **at least once**: a batch can be emitted again after a crash, event `id`s are deterministic (`<blockId>:<table>:<index>`) so receivers can deduplicate. An output only returns once its data is durably delivered, and only then is the cursor checkpoint advanced.

Reorgs are delivered as ordinary events: for each reverted event a tombstone (`op=d`) with `revert_of` set to the reverted event's `id`.

## `stdout://` (or `-`)

One JSON line per event on standard output.

## `file://<dir>`

JSON lines (one event per line) written to local files, rotated by block range.

```
file://./out?blocks=1000&prefix=transfers_
```

| Parameter  | Default | Description                                                                                          |
| ---------- | ------- | ---------------------------------------------------------------------------------------------------- |
| `blocks`   | `1000`  | Blocks per file. Files cover aligned ranges `[k*blocks, (k+1)*blocks)`.                              |
| `prefix`   |         | Prepended to every file name.                                                                        |
| `finalize` | `false` | On exit, turn the unfinished file into a final one named after the blocks it holds.                  |

* File being written: `<prefix><start>-<end>.part` (zero-padded, inclusive bounds of the aligned range). Every batch is `fsync`ed before the cursor advances.
* Once the stream moves past the range, the file is renamed to `.jsonl` (atomic, directory synced). A `.jsonl` file is complete and never changes.
* After a crash or a clean stop the `.part` file is continued by the next run, unless `finalize=true` renamed it on a clean stop. Readers should ignore `.part` files.
* Compensation tombstones are appended to the file being written, files form an append-only log: a reader applies `revert_of` itself. A block replayed after a restart appears twice.
* Local filesystem only (`gs://`, `s3://` have no append nor atomic rename).

## `webhook://host/path` and `webhooks://host/path`

POSTs JSON to `http://` / `https://`. One request per block (or per undo signal), built on the webhook sink client: exponential backoff, retries on network errors and 5xx, no retry on 4xx.

```
webhooks://example.com/hook?auth-header=X-Api-Key&auth=secret&header.X-Env=prod&max-events=500
```

| Parameter        | Default | Description                                                                                       |
| ---------------- | ------- | ------------------------------------------------------------------------------------------------- |
| `timeout`        | `30s`   | Timeout of one call.                                                                              |
| `max-retries`    | `-1`    | Retries after the first attempt, `-1` retries forever so no event is ever dropped.                |
| `max-interval`   | `30s`   | Maximum wait between attempts.                                                                    |
| `max-events`     | `500`   | Maximum events per request, a bigger block is split over several requests, in order.              |
| `max-bytes`      | `4MiB`  | Approximate maximum body size per request (a single bigger event is sent alone).                  |
| `auth-header`    | `Authorization` | Name of the header carrying `auth`.                                                       |
| `auth`           |         | Value of the auth header.                                                                         |
| `signing-secret` |         | Adds `X-Substreams-Signature` (HMAC-SHA256, same scheme as the webhook sink).                     |
| `header.<Name>`  |         | Extra header `<Name>` sent on every request.                                                      |

Other query parameters are forwarded to the receiver. Basic auth in the URL user info works too. Secrets in the URL are visible in the process list, prefer a wrapper that injects them.

Body:

```json
{"block": {"number": 1, "id": "..", "timestamp": ".."}, "cursor": "...", "compensation": false, "part": 1, "parts": 2, "events": [ {..event..} ]}
```

`compensation` is true for undo-signal batches; `part`/`parts` are only present when split. Requests are sent strictly one after the other, so the receiver sees events in stream order. A response outside 2xx after the retries (any 4xx right away) fails the sink with the error, the cursor is not advanced, nothing is dropped; the next run re-sends.

## `pubsub://<project>/<topic>`

One Google Cloud Pub/Sub message per event, data = event JSON. Uses application default credentials, or the emulator when `PUBSUB_EMULATOR_HOST` is set.

```
pubsub://my-project/cdc-events?ordering=key
```

Attributes: `op`, `table`, `block_num`, `id`, and `revert_of` on compensation tombstones.

| Parameter      | Default | Description                                                          |
| -------------- | ------- | -------------------------------------------------------------------- |
| `ordering`     | `table` | Ordering key: `table`, `key` (table + primary key) or `none`.        |
| `concurrency`  | `8`     | Ordering keys published to at the same time.                         |
| `timeout`      | `30s`   | Timeout of one publish attempt.                                      |
| `max-retries`  | `-1`    | Retries, `-1` forever.                                               |
| `max-interval` | `30s`   | Maximum wait between attempts.                                       |

**Ordering.** Pub/Sub only orders messages sharing an ordering key, and only for subscriptions with message ordering enabled.

* `table` (default): all changes of a table are in stream order. One ordering key is limited to about 1 MB/s.
* `key`: changes of a row are in order, rows are independent: more throughput, no order across rows.
* `none`: no order.

A compensation tombstone has the table and primary key of the event it reverts, so under `table` and `key` it is on the same ordering key and is delivered after the event it reverts. Within a batch, events of one ordering key are published in order, different keys in parallel; the emitter waits for every acknowledgement before the cursor advances. A failed publish is retried until it succeeds (ordering key resumed between attempts), duplicates are possible after a crash.
