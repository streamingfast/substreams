# Definition

Substreams introduce a few new concepts to The Graph ecosystem, inspired by traditional large-scale data systems, fused with the novelties of blockchain.

Substreams **is**:

* A streaming-first system
  * Based on gRPC and protobuf
  * Based on the StreamingFast Firehose
* A remote code execution framework, that is:
  * highly cacheable
  * highly parallelizable
* Composable down to individual modules, and allows a community to build higher-order modules with great ease
* Deterministic, as it feeds from deterministic blockchain data

Substreams **is not**:

* A relational database
* A REST service
* Concerned directly with how the data is stored
* A general-purpose non-deterministic event stream processor

The **word** _Substreams_ refers to:

* A plurality of _streams_, each in the form of a _module_.
* Packed in a single package, but streamable individually (a _sub_unit of a package)
* _Streams_ composed from imported modules, blended, enriched or refined together (as in _sub_ or downstream component).
* A wink to Subgraphs
* A manifest or package will usually contain more than one module, and/or import one or more modules. It is therefore fitting to talk about a package being a _Substreams_ package.

The Substreams engine is completely agnostic of the underlying blockchain protocol, and works solely on _data_ extracted from nodes using the Firehose. Different protocols have different chain-specific extensions (e.g. Ethereum, which exposes `eth_call`s).
