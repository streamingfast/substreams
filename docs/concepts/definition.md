---
description: Definition of StreamingFast Substreams
---

# What is Substreams

### What is Substreams?

Substreams is an exceptionally powerful processing engine capable of consuming streams of rich blockchain data. Substreams refines and shapes the data for painless digestion by end-user applications, such as decentralized exchanges.

### Benefits of Substreams

* **Store & Process Blockchain Data**\
  Substreams can be used to populate any kind of data store and also employ extremely powerful parallelization techniques to process huge, ever-growing, blockchain histories.
* **Save Time & Money**\
  Substreams can be scaled horizontally resulting in a massive reduction of processing time, and ultimately cost, through the addition of multiple machines.
* **Community Effort**\
  Communities can combine Substreams data refinement strategies to form compounding levels of data richness and availability.
* **Brand New Web3 Concepts**\
  Substreams brings a handful of new concepts to the wider ecosystem surrounding The Graph and its subgraphs.
* **Blockchain Infused Large-scale Data**\
  Substreams was inspired by traditional large-scale data systems now _fused_ with the novelties of blockchain.
* **Rust & Protobufs**\
  Substreams are defined in modules written in the Rust programming language and utilize Google Protocol Buffer technology.
* **Reduced Archive Node Reliance**\
  Substreams results in reduced reliance on archive nodes!

### Capabilities of Substreams

#### _Substreams **is:**_

* a streaming-first system based on gRPC, protobuf, and StreamingFast Firehose,
* a highly cacheable and parallelizable remote code execution framework,
* composable down to individual modules,
* a solution that enables the community to build higher-order modules with great ease,
* being fed by deterministic blockchain data and is therefore deterministic.

### Limitations of Substreams

#### _Substreams is**:**_

* _not_ a relational database,
* _not_ a REST service,
* _not_ concerned directly with how data is queried,
* _not_ a general-purpose _non-deterministic_ event stream processor.

### **Substreams Deep Dive**

#### Composition Through Community

Substreams enables blockchain developers to write Rust modules that compose data streams alongside the community. The end result of community-developed solutions provides far more meaningful blockchain data than ever before.

#### Parallelization

Substreams provides extremely high-performance indexing by virtue of parallelization, in a streaming-first fashion. These powerful parallelization techniques enable efficient processing of enormous blockchain histories.

#### Horizontally Scalable

Substreams is horizontally scalable presenting the opportunity to reduce processing time simply by adding more computing power, or machines.

#### Substreams & Firehose

Substreams has all the benefits of Firehose, like low-cost caching and archiving of blockchain data, high throughput processing, and cursor-based reorgs handling.

The Substreams _engine_ is completely agnostic of underlying blockchain protocols and works solely on data extracted from nodes using the Firehose.

For example, different protocols have different chain-specific extensions, such as Ethereum, which expose `eth_calls`.

#### Substreams & Sparkle

Substreams is the successor of [StreamingFast Sparkle](https://github.com/streamingfast/sparkle). The current Substreams iteration enables greater composability, provides similar powers of parallelization. Basically, Substreams is a _much_ simpler model to work with.

#### Quick Facts

The _word_ Substreams refers to:

* a wink to Subgraphs,
* a plurality of _streams_, each in the form of a _module,_
* packed in a single package, but streamable individually as a \_sub\_unit of a package,
* _streams_ composed from imported modules, blended, enriched or refined together (as in _sub_ or downstream component),
* a manifest or package will usually contain more than one module, and/or import one or more modules. It is therefore fitting to talk about a package being a _Substreams_ package.

### Problem

Web3 development has been highly centered around saving data to blockchain ledgers. Everything from buying Bitcoin or an NFT to trading cryptocurrencies is rooted in committing transaction data to blockchain ledgers.

Searching through the linear transaction data in the ledgers hasn’t historically seen the same level of development effort. Finding and aggregating blockchain data can be difficult, time-consuming, costly, and computationally intensive. Before Substreams was created, blazing fast, easy and efficient searchability of blockchain data was simply not possible.

### Solution

A revolutionary approach to data extraction from blockchain nodes, called Firehose, provides massive levels of previously unseen data availability to Substreams. Requests can be made for single blocks at any point in the blockchain ledger. The data inside each block is fully searchable down to the transaction event level. Substreams processes many blocks at once, in parallel, enabling developers to instantly isolate and locate any data in full blockchain ledgers without the need for linear processing.

The Rust programming language is used by the developer to define data of interest available in the blockchain. Substreams can route data to a myriad of stores including file systems, relational databases, and even straight into an application’s user interface.

### Substreams vs Other Streaming Engines

Substreams is a streaming engine, that can be compared to Fluvio, Kafka, Apache Spark, RabbitMQ and other such technologies, where a blockchain node (a deterministic data source) acts as the _producer_.

It has a logs-based architecture (the Firehose), and allows for user-defined custom code to be sent to a Substreams server(s), for streaming and/or ad-hoc querying of the available data.
