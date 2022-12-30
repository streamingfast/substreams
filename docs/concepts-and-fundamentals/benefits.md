---
description: StreamingFast Substreams benefits and comparisons
---

# Benefits and comparisons

## Substreams is:

* a streaming-first system based on gRPC, protobuf, and the StreamingFast Firehose,
* a highly cacheable and parallelizable remote code execution framework,
* a solution enabling the community to build higher-order modules,
* composable down to individual modules,
* being fed by deterministic blockchain data and is therefore deterministic.
* _**not**_ a relational database,
* _**not**_ a REST service,
* _**not**_ concerned directly about how data is queried,
* not a general-purpose _non-deterministic_ event stream processor.

### Benefits&#x20;

* **Store and process blockchain data**\
  Substreams uses advanced parallelization techniques to efficiently process large, constantly expanding blockchain histories. The processed data is then available for populating various types of datastores or real-time systems.
* **Streaming-first**\
  Substreams inherit from the extremely low latency extraction provided by the underlying Firehose, making it the fastest blockchain indexing technology on the market.
* **Save time and money**\
  You can horizontally scale Substreams, significantly reducing processing time and increasing efficiency by reducing wait time and missed opportunities.
* **Community effort and composability**\
  Communities are able to combine Substreams modules to form compounding levels of data richness and refinement.
* **Protobuf**\
  ****Substreams uses the power of the protobuf ecosystem, for data modeling and integration for a large number of programming languages.
* **Rust**\
  Substreams modules are written in the Rust programming language, by using a wide array of third-party libraries compilable to WASM, to manipulate blockchain data on-the-fly.
* **Blockchain infused Large-scale data**\
  Substreams was inspired by conventional large-scale data systems _fused_ with the novelties of blockchain.

### Comparison to other engines

Substreams is a streaming engine similar to Fluvio, Kafka, Apache Spark, and RabbitMQ, where a blockchain node serving as a deterministic data source acts as the producer.

Substreams has a logs-based architecture through Firehose, which allows users to send custom code for streaming and ad hoc querying of the available data.

### **Other features**

#### Composition through community

Substreams enables you to write Rust modules composing data streams alongside the community. The end result of community-developed solutions provides far more meaningful blockchain data than ever before.

#### Parallelization

Substreams provides extremely high-performance indexing by virtue of parallelization, in a streaming-first fashion. These powerful parallelization techniques enable the efficient processing of enormous blockchain histories.

#### Horizontally scalable

Substreams is horizontally scalable presenting the opportunity to reduce the processing time by adding more computing power, or machines.

#### Substreams and Firehose

Substreams has all the benefits of Firehose, including low-cost caching and archiving of blockchain data, high throughput processing, and cursor-based reorgs handling.

The Substreams _engine_ is completely platform-independent of underlying blockchain protocols and works solely on data extracted from nodes by using Firehose.

For example, different protocols have different chain-specific extensions, such as Ethereum, which expose `eth_calls`.

###
