---
description: StreamingFast Substreams benefits and comparisons
---

# Benefits and comparisons

## Substreams:

* Is a streaming-first system based on gRPC, protobuf, and the StreamingFast Firehose
* Is a highly cacheable and parallelizable remote code execution framework
* Enables the community to build higher-order modules that are composable down to individual modules
* Is fed by deterministic blockchain data and is therefore deterministic
* Is not a relational database
* Is not a REST service
* Is not concerned directly about how data is queried
* Is not a general-purpose non-deterministic event stream processor

### Substreams offers several benefits:

* The ability to store and process blockchain data using advanced parallelization techniques, making the processed data available for various types of datastores or real-time systems.
* A streaming-first approach that inherits low latency extraction from StreamingFast Firehose, making it the fastest blockchain indexing technology on the market.
* The ability to save time and money by horizontally scaling Substreams and increasing efficiency by reducing processing time and wait time.
* The ability for communities to combine Substreams modules to form compounding levels of data richness and refinement.
* The use of the protobuf ecosystem for data modeling and integration in a variety of programming languages.
* The use of the Rust programming language and a wide array of third-party libraries compilable to WASM to manipulate blockchain data on-the-fly.
* Inspiration from conventional large-scale data systems fused with the novelties of blockchain.

### Comparison to other engines

Substreams is a streaming engine similar to [Fluvio](https://www.fluvio.io/), [Kafka](https://kafka.apache.org/), [Apache Spark](https://spark.apache.org/), and [RabbitMQ](https://www.rabbitmq.com/), where a blockchain node serving as a deterministic data source acts as the producer. Its logs-based architecture through Firehose allows users to send custom code for streaming and ad hoc querying of the available data.

### **Other features**

#### Composition through community

Substreams allows you to write Rust modules that compose data streams alongside the community. The end result of these community-developed solutions is more meaningful blockchain data.

#### Parallelization

Substreams' powerful parallelization techniques enable efficient processing of enormous blockchain histories, providing extremely high-performance indexing in a streaming-first fashion.

#### Horizontally scalable

Substreams is horizontally scalable, offering the opportunity to reduce processing time by adding more computing power or machines.

#### Substreams and Firehose

Substreams offers all the benefits of Firehose, including low-cost caching and archiving of blockchain data, high throughput processing, and cursor-based reorg handling. It is platform-independent of underlying blockchain protocols and works solely on data extracted from nodes using Firehose. For example, different protocols have different chain-specific extensions, such as Ethereum's `eth_calls`."

###
