---
description: StreamingFast Substreams benefits and comparisons
---

# Benefits and comparisons

### Substreams is:

* a streaming-first system based on gRPC, protobuf, and the StreamingFast Firehose,
* a highly cacheable and parallelizable remote code execution framework,
* a solution that enables the community to build higher-order modules with great ease,
* composable down to individual modules,
* being fed by deterministic blockchain data and is therefore deterministic.
* _**not**_ a relational database,
* _**not**_ a REST service,
* _**not**_ concerned directly with how data is queried,
* not a general-purpose _non-deterministic_ event stream processor.

### Benefits&#x20;

* **Store and Process Blockchain Data**\
  Substreams employs extremely powerful parallelization techniques to process huge, ever-growing blockchain histories. It can then be used to populate any kind of data store or real-time system.
* **Streaming-First**\
  Substreams inherit from the extremely low latency extraction provided by the underlying Firehose, making it the fastest blockchain indexing technology on the market.
* **Save Time and Money**\
  Substreams can be scaled horizontally resulting in a massive reduction of processing time, saving wait time and lost opportunities.
* **Community Effort and Composability**\
  Communities can combine Substreams modules to form compounding levels of data richness and refinement.
* **Protobuf**\
  ****Substreams uses the power of the protobuf ecosystem, for data modeling and integration with a large number of programming languages.
* **Rust**\
  Substreams modules are written in the Rust programming language, using a wide array of third-party libraries that compile to WASM, to manipulate blockchain data on-the-fly.
* **Blockchain Infused Large-scale Data**\
  Substreams was inspired by traditional large-scale data systems now _fused_ with the novelties of blockchain.

### Comparison to other engines

Substreams is a streaming engine, that can be compared to Fluvio, Kafka, Apache Spark, RabbitMQ, and other such technologies, where a blockchain node (a deterministic data source) acts as the _producer_.

It has a logs-based architecture, Firehose, and allows for user-defined custom code to be sent to Substreams, for streaming and/or ad hoc querying of the available data.

### **Other features**

#### Composition through community

Substreams enables blockchain developers to write Rust modules that compose data streams alongside the community. The end result of community-developed solutions provides far more meaningful blockchain data than ever before.

#### Parallelization

Substreams provides extremely high-performance indexing by virtue of parallelization, in a streaming-first fashion. These powerful parallelization techniques enable the efficient processing of enormous blockchain histories.

#### Horizontally scalable

Substreams is horizontally scalable presenting the opportunity to reduce the processing time by adding more computing power, or machines.

#### Substreams and Firehose

Substreams has all the benefits of Firehose, like low-cost caching and archiving of blockchain data, high throughput processing, and cursor-based reorgs handling.

The Substreams _engine_ is completely agnostic of underlying blockchain protocols and works solely on data extracted from nodes using the Firehose.

For example, different protocols have different chain-specific extensions, such as Ethereum, which expose `eth_calls`.

###
