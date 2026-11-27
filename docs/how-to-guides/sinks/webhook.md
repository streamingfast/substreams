# Webhook

`substreams sink webhook` sends the output of any module to an HTTP endpoint as a JSON `POST`, one call per block or per batch of blocks.

```bash
substreams sink webhook https://api.example.com/webhook ./my-substreams.spkg map_events -e <endpoint>
```

## Payload

By default, each call carries one block:

```json
{
  "clock": {"number": 12000000, "id": "0xabc", "timestamp": "2024-01-01T00:00:00Z"},
  "manifest": {"moduleName": "map_events", "type": "sf.example.v1.Events"},
  "data": {}
}
```

`data` is the block's module output as JSON. With `--webhook-batch-max-blocks=N`, each call carries up to N blocks in ascending order, and `--webhook-batch-max-bytes` caps the body:

```json
{
  "manifest": {"moduleName": "map_events", "type": "sf.example.v1.Events"},
  "blocks": [{"clock": {}, "data": {}}]
}
```

The [PubSub sink](./pubsub.md) publishes this same batch JSON.

## Authentication and signing

- When the environment variable named by `--webhook-auth-header-value-envvar` (`WEBHOOK_AUTH_HEADER_VALUE` by default) is set, its value is sent in the `--webhook-auth-header-name` header (`Authorization` by default). For example, `Bearer <token>`.
- When the environment variable named by `--webhook-signing-secret-envvar` (`WEBHOOK_SIGNING_SECRET` by default) is set, every call carries an `X-Substreams-Signature: t=<unix seconds>,v1=<hex>` header. `<hex>` is the HMAC-SHA256 of `<t>.<body>`, keyed with the secret.

## Retries and failures

Network errors and 5xx responses are retried with exponential backoff, up to `--webhook-max-retries` (`3` by default, `-1` retries until the process stops). `--webhook-timeout` bounds one call. 3xx and 4xx responses are not retried, and redirects are not followed.

After the last retry, `--webhook-on-failure` decides what happens:

- `skip` (the default) drops the block and continues.
- `exit` keeps the payload in `<state-file>.pending`, writes the reason to `--webhook-termination-log` when that file already exists, and exits with status 75. The next start delivers the pending payload before it connects to Substreams.

The cursor is stored in `--state-file` (`./state.cursor` by default). Your endpoint must accept duplicate calls: a block can be sent again after a restart.

## Reorgs

- `--final-blocks-only` delivers only final blocks, so the endpoint never sees a reorg.
- `--wait-confirmations=N` holds the last N blocks back, until they are final or N newer blocks have arrived, and absorbs any reorg that fits in them. It adds N blocks of latency, and a deeper reorg is a fatal error.
- Otherwise, `--webhook-undo-url` receives a notification naming the last valid block, then the replacement blocks arrive as regular calls:

```json
{"lastValidBlock": {"number": 11999990, "id": "0xdef"}, "manifest": {"moduleName": "map_events"}}
```

An undo notification is never dropped, even with `--webhook-on-failure=skip`.

Run `substreams sink webhook --help` for the full list of flags.
