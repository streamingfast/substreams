# Substreams Pub/Sub sink

`substreams sink pubsub` publishes the output of any module to a Google Cloud Pub/Sub topic. The message body is the [webhook sink](../webhook)'s JSON, the same body `substreams sink webhook` sends.

## Usage

```bash
# Topic id plus project. Credentials are application default credentials.
substreams sink pubsub --project my-gcp-project events ./substreams.yaml map_events -e <endpoint>

# A full topic path does not need --project.
substreams sink pubsub projects/my-gcp-project/topics/events ./substreams.yaml map_events -e <endpoint>
```

Set `PUBSUB_EMULATOR_HOST` (for example `localhost:8888`) to publish to a Pub/Sub emulator instead.

One message per block carries attribute `type=block`:

```json
{"clock": {"number": 12000000, "id": "0xabc", "timestamp": "2024-01-01T00:00:00Z"},
 "manifest": {"moduleName": "map_events", "type": "sf.example.v1.Events"},
 "data": {}}
```

`--pubsub-batch-max-blocks=N` publishes up to N blocks per message, attribute `type=batch`:

```json
{"manifest": {"moduleName": "map_events", "type": "sf.example.v1.Events"},
 "blocks": [{"clock": {"number": 12000000, "id": "0xabc", "timestamp": "2024-01-01T00:00:00Z"}, "data": {}}]}
```

Message ordering is on. For the webhook JSON, every message uses the output module name as its ordering key. A subscription created with message ordering enabled receives that module's messages in publish order.

`--pubsub-undo` also publishes a reorg notification on the same topic, attribute `type=undo`:

```json
{"lastValidBlock": {"number": 11999990, "id": "0xdef"}, "manifest": {"moduleName": "map_events"}}
```

Without `--pubsub-undo` the cursor still moves back, and the blocks that replace the undone ones are published as usual.

## Modules that emit Publish

A module whose output type is `sf.substreams.sink.pubsub.v1.Publish` is published in the [substreams-sink-pubsub](https://github.com/streamingfast/substreams-sink-pubsub) format. Each `Publish.Message` is one Pub/Sub message. Its bytes and attributes are kept, attribute `Cursor` is set to the sink cursor, and the ordering key is the zero-padded block number and the message index (`000000012_00000`). A reorg is one message with attributes `LastValidBlock`, `Step=Undo`, and `Cursor`, and no body. That reorg message is published whether or not `--pubsub-undo` is set. `--pubsub-batch-max-blocks` does not apply to this module.

Any other module is published as the webhook JSON.

## Failure handling

Publish errors are retried with exponential backoff up to `--pubsub-max-retries` (default 3; `-1` retries until the process stops). `--pubsub-timeout` bounds one attempt.

`--pubsub-on-failure=exit` (the default) keeps the payload in `<state-file>.pending` (default `./state.cursor.pending`), writes the reason to `--pubsub-termination-log` when that file already exists, and exits with status 75. The next start publishes the pending payload before it opens a Substreams stream. No block is dropped. The pending file and the exit status match the webhook sink.

`--pubsub-on-failure=skip` drops a block after the last retry and continues. The block stays dropped once a later block saves its cursor. A restart before that save sends the block again, because the cursor still points at the earlier one. An undo notification is never dropped: it is retried until it is published, and it is not written to the pending file.

The cursor is `<state-file>`. Point it at durable storage: the default is a local path.
