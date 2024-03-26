A Substreams package defines the data you want to extract from the blockchain. Then, you can consume that data by using one of the many sinks available. Sinks are integrations that allow you to send the extracted data to different destinations, such as a SQL database, a file or a subgraph.

Some of the sinks are officially supported by one or several Substreams providers (i.e. active support is provided), but other sinks are community-driven and support can't be guaranteed.

## Official

| Name      | Support | Maintainer       | Source Code |
|-----------|---------|------------------|-------------|
| SQL       | O       | StreamingFast    |[substreams-sink-sql](https://github.com/streamingfast/substreams-sink-sql)|
| Go SDK    | O       | StreamingFast    |[substreams-sink](https://github.com/streamingfast/substreams-sink)|
| Rust SDK  | O       | StreamingFast    |[substreams-sink-rust](https://github.com/streamingfast/substreams-sink-rust)|
| JS SDK    | O       | StreamingFast    |[substreams-js](https://github.com/substreams-js/substreams-js)|
| KV Store  | O       | StreamingFast    |[substreams-sink-kv](https://github.com/streamingfast/substreams-sink-kv)|
| Prometheus| O       | Pinax            |[substreams-sink-prometheus](https://github.com/pinax-network/substreams-sink-prometheus)|
| Webhook   | O       | Pinax            |[substreams-sink-webhook](https://github.com/pinax-network/substreams-sink-webhook)|
| CSV       | O       | Pinax            |[substreams-sink-csv](https://github.com/pinax-network/substreams-sink-csv)|
| PubSub    | O       | StreamingFast    |[substreams-sink-pubsub](https://github.com/streamingfast/substreams-sink-pubsub)|

## Community

| Name      | Support | Maintainer       | Source Code |
|-----------|---------|------------------|-------------|
| MongoDB   | C       | Community        |[substreams-sink-mongodb](https://github.com/streamingfast/substreams-sink-mongodb)|
| Files     | C       | Community        |[substreams-sink-files](https://github.com/streamingfast/substreams-sink-files)|

* O = Official Support (by one of the main Substreams providers)
* C = Community Support
