## PubSub

The PubSub integration allows you to send blockchain data to a [Google PubSub](https://cloud.google.com/pubsub?hl=en) topic by emitting a specific Protobuf object in your Substreams: [sf.substreams.sink.pubsub.v1.Publish](https://github.com/streamingfast/substreams-sink-pubsub/blob/develop/proto/sf/substreams/sink/pubsub/v1/pubsub.proto).

### Getting Started

If you are new Substreams, refer to the [Develop Substreams](../../develop/develop.md) section to learn about the main pieces of building a Substreams from scratch.

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