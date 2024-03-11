A Substreams package defines the data you want to extract from the blockchain. Then, you can consume that data by using one of the many sinks available. Sinks are integrations that allow you to send the extracted data to different destinations, such as a SQL database, a file or a subgraph.

Some of the sinks are officially supported by one or several Substreams providers (i.e. active support is provided), but other sinks are community-driven and support can't be guaranteed.

| Name      | Support | Maintainer       | Source Code |
|-----------|---------|------------------|-------------|
| SQL       | O       | StreamingFast    |[GitHub](https://github.com/streamingfast/substreams-sink-sql)|
| Go SDK    | O       | StreamingFast    |[GitHub](https://github.com/streamingfast/substreams-sink-kv)|
| Rust SDK  | O       | StreamingFast    |[GitHub](https://github.com/streamingfast/substreams-sink)|
| JS SDK    | O       | StreamingFast    |[GitHub](https://github.com/substreams-js/substreams-js)|
| KV Store  | C       | Community        |[GitHub](https://github.com/streamingfast/substreams-sink-kv)|
| KV Store  | O       | Pinax            |[GitHub](https://github.com/pinax-network/substreams-sink-prometheus)|
| KV Store  | C       | Community        |[GitHub](https://github.com/streamingfast/substreams-sink-mongodb
)|
| PubSub    | O       | StreamingFast    |[GitHub](https://substreams.streamingfast.io/documentation/consume/other-ways-of-consuming/pubsub)|
| PubSub    | C       | Community    |[GitHub](https://github.com/streamingfast/substreams-sink-files)|

* O = Official Support
* C = Community Support
