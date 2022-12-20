# Substreams Sinks

## **Introduction**

The data captured from a blockchain with Substreams can be routed to multiple types of sinks. A sink is the final destination for data acquired through Substreams. Examples include databases, a Slack channel, or flat file storage. Sinks have a wide range of types and Substreams data can be routed anywhere a developer can imagine.

StreamingFast provides a few examples, libraries, and tools to assist Substreams developers with routing blockchain data to sinks.

## **Basics**

Data captured and processed by Substreams can be stored in many different ways through sinks. A Substreams developer’s imagination is really the only limitation. Immediate and typical storage types could be a database or flat files however Substreams data can be piped into other desired locations required by a new or existing application or architecture.

An important design aspect of Substreams is the decision to rely on Google Protocol Buffers, also called protobufs, for data packaging and transmission. Protobufs provide a data-centric, technology stack, and language agnostic approach to working with data that is passed from one application to another. The application-agnostic, data centric capabilities of protobufs give developers the opportunity to package and route data captured by Substreams to other sources, including sinks.

At a low-level Substreams consumes data through a gRPC streaming service. Consumers receive streams of data scoped to a single block as requests are sent.

## **General Requirements**

The first step of having Substreams consumed by a particular sink involves the creation of a `map` module; whose output type is a protobuf (accepted by the sink). This specific protobuf is populated from Substreams protobuf types; a transformation of types into a format suitable for ingestion by sinks.

For example, database-like Substreams sinks such as PostgreSQL or MongoDB accept a module's output of type [substreams.database.v1.DatabaseChanges](https://github.com/streamingfast/substreams-database-change/blob/develop/proto/database/v1/database.proto#L5).

It’s important to note that databases are only one type of sink. The sink being targeted determines what output type should be respected.

The sink reads the specific protobuf-based data being sent out of Substreams and performs the processing for it. Every sink performs differently regarding the data received, most will perform some kind of storage.

The configuration of this storage layer and the requirements of it the responsibility of the Substreams developer. Each Substreams sink should document the specific infrastructure required for running the `sink`. Read the documentation about each sink to understand its behavior and requirements.

An understanding of basic Substreams fundamentals is expected before continuing. Learn more about modules in the Substreams documentation.

[https://substreams.streamingfast.io/concept-and-fundamentals/modules](https://substreams.streamingfast.io/concept-and-fundamentals/modules)

## **Existing & Commnuity Sinks**

StreamingFast values external contributions for Substreams sinks. If your team has created a sink, please contact the StreamingFast team [through Discord](https://discord.gg/mYPcRAzeVN) so we can add it to the documentation!

The `substreams-eth-block-meta` example demonstrates sinks in action. Check out the source code in the project’s official GitHub repository.

[https://github.com/streamingfast/substreams-eth-block-meta](https://github.com/streamingfast/substreams-eth-block-meta)

StreamingFast provides several tools to assist Substreams developers interested in persisting data to databases; each can be found in its official GitHub repository.

**PostgreSQL**

[https://github.com/streamingfast/substreams-sink-postgres](https://github.com/streamingfast/substreams-sink-postgres)

**MongoDB**

[https://github.com/streamingfast/substreams-sink-mongodb](https://github.com/streamingfast/substreams-sink-mongodb)

**File Based Storage**

[https://github.com/streamingfast/substreams-sink-files](https://github.com/streamingfast/substreams-sink-files)

## **Build a Sink**

StreamingFast provides tools allowing developers to route blockchain data to a few different types of data storage sinks, or means of ingestion. The types of sinks with tools provided by StreamingFast aren’t the only options for Substreams developers. Existing applications, databases, and other tools can be fed by blockchain data captured and output by Substreams.

Developers can get inspiration on how to write their own sink by looking at sinks provided by StreamingFast, today. One example could be a database, such as Oracle, that doesn’t currently have tools in place. Developers should be able to review the [PostgreSQL Sink](https://github.com/streamingfast/substreams-sink-postgres) tool and its codebase to understand how to construct a custom data-sinking solution.

Reiterating from above, protobufs are designed by the developer. The protobufs are used to transfer data out of Substreams to the data sink. Protobufs aren’t tied to any particular technology stack or language, enabling developers to capture, further process, use and store data provided by Substreams in a myriad of different capacities.

Through careful design of the Substreams manifest, modules, and protobufs developers can craft their output data in many ways. One option, as seen in the PostgreSQL example is through a single output protobuf. The flexibility of Substreams design however allows for other strategies, including multiple protobufs and modules.

Developers need to examine and account for the format and any requirements of the end target they want their data routed. The specifics of how data is ingested by the targeted sink will determine the design of the output from Substreams.
