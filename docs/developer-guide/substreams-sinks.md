# Substreams Sinks

## **Introduction**

The data captured from a blockchain with Substreams can be routed to multiple types of sinks. A sink is an end container to house the data acquired. Examples are a database, Slack channel, or flat file storage. Sinks have a wide range of types and Substreams data can be routed anywhere a developer can imagine.

StreamingFast provides a few examples, libraries, and tools to assist Substreams developers with routing blockchain data to sinks.

## **Basics**

Data captured and processed by Substreams can be stored in many different ways through sinks. A Substreams developer’s imagination is really the only limitation. Immediate and typical storage types could be a database or flat files however Substreams data can be piped into other desired locations required by a new or existing application or architecture.

An important design aspect of Substreams is the deciiosn to rely on Google Protocol Buffers, or protobufs, for data packaging and transmission. Protobufs provide a data-centric, technology stack and languages agnostic approach to working with data that is passed from one application to another. The application-agnostic capabilities of protobufs give developers the opportunity to package and route data captured by Substreams to other sources, including sinks.

## **General Requirements**

One of the critical steps involved is the creation of a protobuf that forms data to meet the requirements of a sink. The protobuf is populated with blockchain data captured in a Substreams module and then used as output. Existing sink solutions, such as PostgreSQL, provided by StreamingFast, demonstrate this functionality. It’s important to note that databases are merely one type of sink.

The consuming application, or code, can read protobuf-based data being sent out of Substreams. Protobufs are flexible and the expectations of the consuming application can be matched closely with mindful data design. Substreams will send the data through a map module using a protobuf defined by the developer. The data is then consumed by another application that will route the data to the desired location, or sink.

An understanding of basic Substreams fundamentals is suggested before continuing. Learn more about modules basics in the Substreams documentation at the following link.

https://substreams.streamingfast.io/concept-and-fundamentals/modules

## **Existing & Commnuity Sinks**

StreamingFast values external contributions for Substreams sinks. If your team has created a sink, please reach so we can add it to the documentation!

## **Build a Sink**

StreamingFast provides tools allowing developers to route blockchain data to a few different types of data storage sinks, or means of ingestion. The types of sinks with tools provided by StreamingFast aren’t the only options for Substreams developers. Existing applications, databases, and other tools can be fed by blockchain data captured and output by Substreams.

Developers can examine the StreamingFast sink tools to see examples of how protobuf-based data from Substreams can be used with other approaches. One example could be a database, such as Oracle, that doesn’t currently have tools in place. As mentioned, databases are only one example of where Substreams data can be routed. Developers should be able to review the PostgreSQL tool and its codebase to begin to understand how to construct their own data-sinking solution.

Reiterating from above, protobufs are designed by the developer. The protobufs are used to transfer data out of Substreams to the data sink. Protobufs aren’t tied to any particular technology stack or language, enabling developers to capture, further process, use and store the Substreams data in a myriad of different capacities.

Through careful design of the Substreams manifest, modules, and protobufs developers can craft their output data in many ways. One option, as seen in the PostgreSQL example is through a single output protobuf. The flexibility of Substreams design however allows for other strategies, including multiple protobufs and modules. Developers need to examine and account for the format and any requirements of the end target they want their data routed. The specifics of how data is ingested by the targeted sink will determine the design of the output from Substreams.

The substreams-eth-block-meta example demonstrates sinks in action. Check out the source code in the project’s official GitHub repository.

https://github.com/streamingfast/substreams-eth-block-meta

StreamingFast provides several tools to assist Substreams developers interested in persisting data to databases; each can be found in its official GitHub repository.

**PostgreSQL**
https://github.com/streamingfast/substreams-sink-postgres

**MongoDB**
https://github.com/streamingfast/substreams-sink-mongodb

**File Based Storage**
https://github.com/streamingfast/substreams-sink-files
