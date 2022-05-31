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

The Substreams engine is completely agnostic of the underlying blockchain protocol, and works solely on _data_ extracted from nodes using the Firehose. Different protocols have chain-specific extensions, like Ethereum, which exposes `eth_call`s.
