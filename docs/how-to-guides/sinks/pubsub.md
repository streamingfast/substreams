## PubSub

`substreams sink pubsub` publishes the output of any module to a [Google Cloud Pub/Sub](https://cloud.google.com/pubsub) topic. The message body is the [webhook sink](https://github.com/streamingfast/substreams/tree/develop/sink/webhook)'s JSON, the same body `substreams sink webhook` sends.

```bash
substreams sink pubsub --project my-gcp-project my-topic ./my-substreams.spkg map_events -e <endpoint>
```

`<topic>` is a topic id, used with `--project`, or a full `projects/<project>/topics/<topic>` path. Credentials are [application default credentials](https://cloud.google.com/docs/authentication/application-default-credentials). When `PUBSUB_EMULATOR_HOST` is set, the sink publishes to that emulator.

Each block is one message with attribute `type=block`:

```json
{
  "clock": {"number": 12000000, "id": "0xabc", "timestamp": "2024-01-01T00:00:00Z"},
  "manifest": {"moduleName": "map_events", "type": "sf.example.v1.Events"},
  "data": {}
}
```

`--pubsub-batch-max-blocks=N` puts up to N blocks in one message (`type=batch`). `--pubsub-batch-max-bytes` caps that body, and `--pubsub-batch-max-wait` bounds how long a batch waits.

Message ordering is enabled. Every message uses the output module's name as its ordering key, so a subscription created with message ordering enabled receives that module's messages in publish order.

`--pubsub-undo` publishes a reorg notification on the same topic (`type=undo`) naming the last valid block. Without it, the cursor still moves back and the blocks that replace the undone ones are published as usual.

The cursor is stored in `--state-file` (`./state.cursor` by default). A payload that could not be published is kept in `<state-file>.pending`, the reason is written to `--pubsub-termination-log` when that file already exists, and the process exits with status 75 (`--pubsub-on-failure=exit`, the default). The next start publishes that payload before it connects to Substreams. The pending file and the exit status match the webhook sink. `--pubsub-on-failure=skip` drops a block after the last retry and continues. An undo notification is never dropped.

### Getting Started

If you are new to Substreams, start with [Develop Substreams](../../how-to-guides/develop-your-own-substreams/develop-your-own-substreams.md).

Create a topic in Google Cloud Pub/Sub, then publish any module to it with `substreams sink pubsub`:

```bash
substreams sink pubsub --project <project_id> <topic_name> <substreams_manifest> <substreams_module_name> -e <endpoint>
```

- `endpoint`: the Substreams endpoint. [Chains & Endpoints](../../references/chains-and-endpoints.md) lists them.
- `project_id`: the Google Cloud project ID. Omit `--project` when `<topic_name>` is a full `projects/<project>/topics/<topic>` path.
- `substreams_manifest`: path to the Substreams manifest or package.
- `substreams_module_name`: the output module. Any module works. Its output is published as the webhook sink's JSON, the same body `substreams sink webhook` sends.
- `topic_name`: the Pub/Sub topic ID.

### Modules that emit Publish

The standalone [`substreams-sink-pubsub`](https://github.com/streamingfast/substreams-sink-pubsub) binary remains for a module that emits [sf.substreams.sink.pubsub.v1.Publish](https://github.com/streamingfast/substreams-sink-pubsub/blob/develop/proto/sf/substreams/sink/pubsub/v1/pubsub.proto). That message chooses the bytes and attributes of each Pub/Sub message itself.

- Clone the [substreams-sink-pubsub](https://github.com/streamingfast/substreams-sink-pubsub) repository.
- From that checkout, install the CLI:

```bash
go install ./cmd/substreams-sink-pubsub
```

- Create a topic in Google Cloud Pub/Sub.
- Run the sink. The module must emit `sf.substreams.sink.pubsub.v1.Publish`:

```bash
substreams-sink-pubsub sink -e <endpoint> --project <project_id> <substreams_manifest> <substreams_module_name> <topic_name>
```

Examples of modules that emit `Publish` are in the `examples` directory of that repository.