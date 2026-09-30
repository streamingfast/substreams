## PubSub

`substreams sink pubsub` publishes the output of any module to a [Google Cloud Pub/Sub](https://cloud.google.com/pubsub) topic. The message body is the same JSON that `substreams sink webhook` sends.

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

The cursor is stored in `--state-file` (`./state.cursor` by default). A payload that could not be published is kept in `<state-file>.pending`, the reason is written to `--pubsub-termination-log` when that file already exists, and the process exits with status 75 (`--pubsub-on-failure=exit`, the default). The next start publishes that payload before it connects to Substreams. `--pubsub-on-failure=skip` drops a block after the last retry and continues. An undo notification is never dropped.

### Modules that emit Publish

The standalone [`substreams-sink-pubsub`](https://github.com/streamingfast/substreams-sink-pubsub) binary remains for a module that emits [sf.substreams.sink.pubsub.v1.Publish](https://github.com/streamingfast/substreams-sink-pubsub/blob/develop/proto/sf/substreams/sink/pubsub/v1/pubsub.proto). That message chooses the bytes and attributes of each Pub/Sub message itself.

### Getting Started

If you are new Substreams, refer to the [Develop Substreams](../../how-to-guides/develop-your-own-substreams/develop-your-own-substreams.md) section to learn about the main pieces of building a Substreams from scratch.

- Clone the [https://github.com/streamingfast/substreams-sink-pubsub](https://github.com/streamingfast/substreams-sink-pubsub) GitHub repository.
- Install the PubSub CLI. This CLI will help in deploying your Substreams to the PubSub Service.

```bash
go install ./cmd/substreams-sink-pubsub
```

- Create a topic in the Google PubSub Service, where the data of your Substreams will be sent.
- Deploy your Substreams by using the PubSub CLI:

```bash
substreams-sink-pubsub sink -e <endpoint> --project <project_id> <substreams_manifest> <substreams_module_name> <topic_name>
```
    - `endpoint`: the Substreams provider endpoint that will be used to extract the data (you can find the endpoints available in the [Chains & Endpoints](../../references/chains-and-endpoints.md)) section.
    - `project_id`: ID of the Google project.
    - `substreams_manifest`: path to the Substreams manifest.
    - `substreams_module_name`: name of the Substreams output module. The module must emit `sf.substreams.sink.pubsub.v1.Publish` data.
    - `topic_name`: name of the Google topic where the data will be sent.

You can find some Substreams examples in the `examples` directory of the repository.