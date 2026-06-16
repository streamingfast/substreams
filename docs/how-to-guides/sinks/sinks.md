Once you find a package that fits your needs, you can choose how you want to consume the data. Sinks are integrations that allow you to send the extracted data to different destinations, such as a SQL database, or a file.

{% hint style="info" %}
**Note**: Some of the sinks are officially supported by StreamingFast (i.e. active support is provided), but other sinks are community-driven and support can’t be guaranteed.
{% endhint %}

- [Hosted Sinks](./hosted-sinks/hosted-sinks.md): Let StreamingFast run your sink for you — no infrastructure to manage.
- [SQL Database](./sql/sql.md): Send the data to a database.
- [Direct Streaming](./stream/stream.md): Stream data directly from your application.
- [PubSub](./pubsub.md): Send data to a PubSub topic.
- [Community Sinks](../sinks/community): Explore quality community maintained sinks.

## Navigating Sink Repos

### Official

| Name       | Support | Maintainer       | Source Code |
|------------|---------|------------------|-------------|
| SQL        | O       | StreamingFast    |[substreams-sink-sql](https://github.com/streamingfast/substreams-sink-sql)|
| Go SDK     | O       | StreamingFast    |[substreams-sink](https://github.com/streamingfast/substreams-sink)|
| Rust SDK   | O       | StreamingFast    |[substreams-sink-rust](https://github.com/streamingfast/substreams-sink-rust)|
| JS SDK     | O       | StreamingFast    |[substreams-js](https://github.com/substreams-js/substreams-js)|
| KV Store   | O       | StreamingFast    |[substreams-sink-kv](https://github.com/streamingfast/substreams-sink-kv)|
| PubSub     | O       | StreamingFast    |[substreams-sink-pubsub](https://github.com/streamingfast/substreams-sink-pubsub)|
| ProtoJSON  | O       | StreamingFast    |[substreams-sink-protojson](https://github.com/streamingfast/substreams/tree/develop/sink/protojson)|
| Webhook    | O       | StreamingFast    |[substreams-sink-webhook](https://github.com/streamingfast/substreams/tree/develop/sink/webhook)|
| Noop       | O       | StreamingFast    |[substreams-sink-noop](https://github.com/streamingfast/substreams/tree/develop/sink/noop)|
| Prometheus | O       | Pinax            |[substreams-sink-prometheus](https://github.com/pinax-network/substreams-sink-prometheus)|
| Webhook(JS)| O       | Pinax            |[substreams-sink-webhook](https://github.com/pinax-network/substreams-sink-webhook)|
| CSV        | O       | Pinax            |[substreams-sink-csv](https://github.com/pinax-network/substreams-sink-csv)|

### Community

| Name      | Support | Maintainer       | Source Code |
|-----------|---------|------------------|-------------|
| MongoDB   | C       | Community        |[substreams-sink-mongodb](https://github.com/streamingfast/substreams-sink-mongodb)|
| Files     | C       | Community        |[substreams-sink-files](https://github.com/streamingfast/substreams-sink-files)|
| KV Store  | C       | Community        |[substreams-sink-kv](https://github.com/streamingfast/substreams-sink-kv)|
| Prometheus| C       | Community        |[substreams-sink-Prometheus](https://github.com/pinax-network/substreams-sink-prometheus)|

* O = Official Support (by one of the main Substreams providers)
* C = Community Support
