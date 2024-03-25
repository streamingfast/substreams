A Substreams package defines the data you want to extract from the blockchain. Then, you can consume that data by using one of the many sinks available. Sinks are integrations that allow you to send the extracted data to different destinations, such as a SQL database, a file or a subgraph.

Some of the sinks are officially supported by one or several Substreams providers (i.e. active support is provided), but other sinks are community-driven and support can't be guaranteed.

| Name      | Support | Maintainer       | Source Code |
|-----------|---------|------------------|-------------|
| SQL       | O       | StreamingFast    |[GitHub](https://github.com/streamingfast/substreams-sink-sql)|
| Go SDK    | O       | StreamingFast    |[GitHub](https://github.com/streamingfast/substreams-sink-kv)|
| Rust SDK  | O       | StreamingFast    |[GitHub](https://github.com/streamingfast/substreams-sink)|
| JS SDK    | O       | StreamingFast    |[GitHub](https://github.com/substreams-js/substreams-js)|
| KV Store  | C       | Community        |[GitHub](https://github.com/streamingfast/substreams-sink-kv)|
| Prometheus| O       | Pinax            |[GitHub](https://github.com/pinax-network/substreams-sink-prometheus)|
| Webhook   | O       | Pinax            |[GitHub](https://github.com/pinax-network/substreams-sink-webhook)|
| CSV       | O       | Pinax            |[GitHub](https://github.com/pinax-network/substreams-sink-csv)|
| MongoDB   | C       | Community        |[GitHub](https://github.com/streamingfast/substreams-sink-mongodb)|
| PubSub    | O       | StreamingFast    |[GitHub](https://substreams.streamingfast.io/documentation/consume/other-ways-of-consuming/pubsub)|
| Files     | C       | Community        |[GitHub](https://github.com/streamingfast/substreams-sink-files)|

* O = Official Support (by one of the main Substreams providers)
* C = Community Support
