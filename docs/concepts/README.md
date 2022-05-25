# Concepts & Fundamentals

Substreams introduce a few new concepts to The Graph ecosystem, inspired by traditional large scale data systems and fused with the novelties of blockchain.

## Definition

Substreams **is**:

* A streaming first system
  * Based on gRPC and protobuf
  * Based on the StreamingFast Firehose
* A remote code execution framework, that is:
  * highly cacheable
  * highly parallelizable
* Composable down to individual modules, and allows a community to build higher order modules with great ease (more on that later)

Substreams **is not**:

* A relational database
* A REST service
* Concerned directly with how the data is stored

## Comparison

Substreams is a streaming engine, that can be compared to Fluvio, Kafka, Apache Spark, RabbitMQ and other suchs technologies, where a blockchain node, a deterministic data source, acts as the _producer_.

It has a logs based architecture (the Firehose), and allows for user-defined custom code to be sent to a Substreams server(s), for streaming and/or ad-hoc querying of the available data.

##
