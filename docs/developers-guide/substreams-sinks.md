---
description: StreamingFast Substreams data sinks
---

# Substreams sinks

## **Substreams sinks overview**

You can route the data captured from a blockchain by using Substreams to multiple types of sinks. A sink is a final destination for data acquired through Substreams. Examples include databases, a Slack channel, or flat file storage. Sinks have a wide range of types and Substreams data and you can route data anywhere you're able to imagine.

StreamingFast provides a few examples, libraries, and tools to assist you when routing blockchain data to sinks.

## **Basics**

Databases and flat files are standard storage types however you can pipe Substreams data into other locations required by a new or existing application or architecture.

An important design aspect of Substreams is the decision to rely on Google Protocol Buffers, also called protobufs, for data packaging and transmission.&#x20;

Protobufs provide a data-centric, technology stack, non-language specific, and platform-independent approach to using data that is passed from one application to another.

{% hint style="success" %}
**Tip**: The platform-independent, data-centric capabilities of protobufs give you the opportunity to package and route data captured by Substreams to other sources, including sinks.
{% endhint %}

At a low-level Substreams consumes data through a gRPC streaming service. Consumers receive streams of data scoped to a single block as requests are sent.

## **General requirements**

The first step of having Substreams consumed by a particular sink involves the creation of a `map` module; whose output type is a protobuf (accepted by the sink). The protobuf is populated from Substreams protobuf types; a transformation of types into a format suitable for loading into sinks.

For example, database-like Substreams sinks such as PostgreSQL or MongoDB accept a module's output of type [substreams.database.v1.DatabaseChanges](https://github.com/streamingfast/substreams-database-change/blob/develop/proto/database/v1/database.proto#L5).

{% hint style="success" %}
**Tip**: Databases are only one type of sink. The sink determines the output type to be respected.
{% endhint %}

The sink reads the specific protobuf-based data being sent out of Substreams and performs the processing for it. Every sink performs differently regarding the data received, most perform some kind of storage.

The configuration of the storage layer and its requirements are your responsibility. StreamingFast  provides documentation for the infrastructure required by various Substreams `sinks.` Read the documentation to understand the behavior and requirements for the other `sink` types.

An understanding of basic Substreams fundamentals is expected before continuing. Learn more about modules in the Substreams documentation.

[https://substreams.streamingfast.io/concept-and-fundamentals/modules](https://substreams.streamingfast.io/concept-and-fundamentals/modules)

## **Existing and community sinks**

StreamingFast values external contributions for Substreams sinks. If your team has created a sink, contact the StreamingFast team [through Discord](https://discord.gg/mYPcRAzeVN) so it gets included in the Substreams documentation!

The `substreams-eth-block-meta` example demonstrates sinks in action. Check out the source code in the project’s official GitHub repository.

[https://github.com/streamingfast/substreams-eth-block-meta](https://github.com/streamingfast/substreams-eth-block-meta)

StreamingFast provides several tools to assist database persistence for Substreams.

**PostgreSQL**

[https://github.com/streamingfast/substreams-sink-postgres](https://github.com/streamingfast/substreams-sink-postgres)

**MongoDB**

[https://github.com/streamingfast/substreams-sink-mongodb](https://github.com/streamingfast/substreams-sink-mongodb)

**File-based storage**

[https://github.com/streamingfast/substreams-sink-files](https://github.com/streamingfast/substreams-sink-files)

## **Build a sink**

StreamingFast provides tools allowing you to route blockchain data to a few different types of data storage sinks, or means of importation; however StreamingFast sink tools aren’t the only options. Existing applications, databases, and other tools are fed by blockchain data captured by Substreams.

{% hint style="success" %}
**Tip**: To get inspiration for writing your own sink study the examples provided by StreamingFast. One example is a database, such as Oracle, lacking Substreams sink tools. Study the [PostgreSQL Sink](https://github.com/streamingfast/substreams-sink-postgres) tool and its codebase to understand how to construct a custom data-sinking solution.
{% endhint %}

Protobufs are designed to use for transferring data out of Substreams into the data sink. Protobufs aren’t tied to any particular technology stack or language, enabling you to capture, further process, use and store data provided by Substreams in different capacities.

{% hint style="info" %}
**Note**: Through careful design of the Substreams manifest, modules, and protobufs you can craft your output data in a variety of ways. One option, as seen in the PostgreSQL example is through a single output protobuf. The flexibility of Substreams design however allows for other strategies, including multiple protobufs and modules.
{% endhint %}

You need to examine and account for the format and any requirements of the end environment you want your data routed. The specifics of how data is ingested by the sink determine the design of the output from Substreams.
