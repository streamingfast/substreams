---
description: StreamingFast Substreams benefits and comparisons
---

# Benefits and comparisons

## Important Substreams facts include:

* It provides a streaming-first system based on gRPC, protobuf, and the StreamingFast Firehose.
* It supports a highly cacheable and parallelizable remote code execution framework.
* It enables the community to build higher-order modules that are composable down to individual modules.
* Deterministic blockchain data is fed to Substreams, **making it deterministic**.
* It is **not** a relational database.
* It is **not** a REST service.
* It is **not** concerned directly about how data is queried.
* It is **not** a general-purpose non-deterministic event stream processor.

### Substreams offers several benefits including:

* The ability to store and process blockchain data using advanced parallelization techniques, making the processed data available for various types of data stores or real-time systems.
* A streaming-first approach that inherits low latency extraction from [StreamingFast Firehose](https://firehose.streamingfast.io/).
* The ability to save time and money by horizontally scaling and increasing efficiency by reducing processing time and wait time.
* The ability for communities to [combine Substreams modules](../developers-guide/modules/) to form compounding levels of data richness and refinement.
* The use of [protobufs for data modeling and integration](../developers-guide/creating-protobuf-schemas.md) in a variety of programming languages.
* The use of the Rust programming language and a wide array of third-party libraries compilable with WASM to manipulate blockchain data on-the-fly.
* Inspiration from conventional large-scale data systems fused into the novelties of blockchain technology.

### **Other features**

#### Composition through community

Substreams allows you to write Rust modules that compose data streams alongside the community. The end result of these community-developed solutions is more meaningful blockchain data.

#### Parallelization

Substreams' powerful parallelization techniques enable efficient processing of enormous blockchain histories, providing extremely high-performance indexing in a streaming-first fashion.

#### Horizontally scalable

Substreams is horizontally scalable, offering the opportunity to reduce processing time by adding more computing power or machines.

#### Substreams and Firehose

Substreams offers all the benefits of [Firehose](https://firehose.streamingfast.io/), including low-cost caching and archiving of blockchain data, high throughput processing, and cursor-based reorg handling. It is platform-independent of underlying blockchain protocols and works solely on data extracted from nodes using Firehose. For example, different protocols have different chain-specific extensions, such as Ethereum's `eth_calls`.

### Comparison to other engines

Substreams is a streaming engine similar to [Fluvio](https://www.fluvio.io/), [Kafka](https://kafka.apache.org/), [Apache Spark](https://spark.apache.org/), and [RabbitMQ](https://www.rabbitmq.com/), where a blockchain node serving as a deterministic data source acts as the producer. Its logs-based architecture through [Firehose](https://firehose.streamingfast.io/) allows users to send custom code for streaming and ad hoc querying of the available data.

#### Substreams & Subgraphs

A lot of questions arise around Substreams and Subgraphs as they are both part of The Graph ecosystem. Substreams has been created by StreamingFast team, the first core developers teams outside of Edge & Node, the founding team of The Graph. It was created in response to different use cases especially around analytics and big data that couldn't be served by Subgraph due to its current programming model. Here some of the key points for which Substreams were created:

- Offer a streaming-first approach to consuming/transforming blockchain's data
- Offer a highly parallelizable yet simple model to consume/transform blockchain's data
- Offer a composable system where you can depend on building blocks offered by the community
- Offer rich block model

While they share similar ideas around blockchain's transformation/processing and they are both part of The Graph ecosystem, both can be viewed as independent technology that are unrelated to each other. One cannot take a Subgraph's code and run it on Substreams engine, they are incompatible. Here some of key differences:

- You write your Substreams in Rust while Subgraph are written in AssemblyScript
- Substreams are "stateless" request through gRPC while Subgraphs are persistent deployment
- Substreams offers you the chain's specific full block while in Subgraph, you define "triggers" that will invoke your code
- Substreams are consumed through a gRPC connection where you control the actual output message while Subgraphs are consumed through GraphQL
- Substreams have no long-term storage nor database (it has transient storage) while Subgraph stores data persistently in Postgres
- Substreams can be consumed in real-time with a fork aware model while Subgraph can only be consumed through GraphQL and polling for "real-time"

Substreams offer quite a different model when compared to Subgraph, just Rust alone is a big shift for someone used to write Subgraphs in AssemblyScript. Substreams is working a lot also with Protobuf models also.

One of the benefits of Substreams is that the persistent storage solution is not part of the technology directly, so you are free to use the database of your choice which enable a lot of analytics use cases that was not possible (or harder to implement) today using Subgraphs like persistent your transformed data to BigQuery or Clickhouse, Kafka, etc. Also, the live streaming feature of Substreams enables further use cases and super quick reactivity that will benefits a lot of user.