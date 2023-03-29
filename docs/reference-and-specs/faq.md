---
description: StreamingFast Substreams frequently asked questions
---

# FAQ

## **Substreams FAQ overview**

You can find answers to common Substreams questions in the FAQ documentation. If the answer you're looking for is not included [contact the StreamingFast team](https://discord.gg/mYPcRAzeVN) through Discord to get help.

### **What is Substreams?**

Substreams is an exceptionally powerful processing engine capable of consuming streams of rich blockchain data. Substreams refines and shapes the data for painless digestion by end-user applications, such as decentralized exchanges.

### **Do I need Firehose to use Substreams?**

Developers do not need a dedicated installation of Firehose to use Substreams. StreamingFast provides a public Firehose endpoint made available to developers.

### **Is it possible to use Substreams in my subgraph?**

Yes, Substreams compliments and extend the capabilities and functionalities of subgraphs. Additional information is available in the [Substreams documentation for graph-node and subgraphs](https://substreams.streamingfast.io/reference-and-specs/graph-node-setup).

### **Is it possible to use Substreams for production deployments?**

No, Substreams is provided as a developer preview.

### **What is the `substreams` CLI?**

The [`substreams` command line interface (CLI)](command-line-interface.md) is the main tool developers use to use the Substreams engine. The [`substreams` CLI](command-line-interface.md) provides a range of features, commands, and flags. Additional information for the [`substreams` CLI](command-line-interface.md) is available in the Substreams documentation.

### **How do I get a Substreams authentication token?**

Authentication tokens are required to use Substreams and connect to the public Firehose endpoint. Full [instructions for obtaining a StreamingFast authentication token](https://substreams.streamingfast.io/reference-and-specs/authentication) are available in the Substreams documentation.

### **My Substreams authentication token isn’t working, what do I do?**

The StreamingFast team is [available on Discord to resolve problems](https://discord.gg/Ugc7KtkA) related to obtaining or by using authentication tokens.

The Substreams documentation also [provides general instructions surrounding authentication](https://substreams.streamingfast.io/reference-and-specs/authentication) tokens.

### **How do I create a Substreams module?**

Developers create their own Substreams implementations in a variety of ways. Use these [examples](reference-and-specs/examples.md) as a reference and starting point.

The Substreams documentation [provides a Developer's guide](https://substreams.streamingfast.io/developer-guide/overview) to assist you to understand and use Substreams.

### **What is Substreams used for?**

Substreams and Firehose work together to index and process blockchain data. Substreams is used for transforming rich blockchain data and exposing it to the needs of application developers.

### **Is Substreams free?**

Yes, Substreams is an open source project and is available to the public for free.

### **How does a developer reach the information returned from a call to Substreams from a web-based UI?**

Substreams is not meant to be piped to a web UI, it’s a data transformation layer. Substreams reaches Subgraphs, as a data source, and makes GraphQL available for web consumption. Other sinks might expose APIs for web browsers, however, it's not the responsibility of Substreams.

### Is it possible to listen for new blocks?

Specifying a stop block value of zero (0), the default enables transparent handoff from historical to real-time blocks.

### **Does StreamingFast have a Discord?**

Yes, [join the StreamingFast Discord](https://discord.gg/Ugc7KtkA).

### **Is StreamingFast on Twitter?**

Yes, [find StreamingFast on their official Twitter account](https://twitter.com/streamingfastio).

### **Is StreamingFast on YouTube?**

Yes, [find StreamingFast on their official YouTube account](https://www.youtube.com/c/streamingfast).

### **Who is dfuse?**

StreamingFast was originally called dfuse. The company changed its name and is in the process of rebranding.

### What is Sparkle?

Substreams is the successor of [StreamingFast Sparkle](https://github.com/streamingfast/sparkle). Substreams enables greater composability, and provides similar parallelization capabilities. Sparkle is deprecated.

### **Who is StreamingFast?**

StreamingFast is a protocol infrastructure company providing a massively scalable architecture for streaming blockchain data. StreamingFast is one of the core developers working alongside The Graph Foundation.

### Why the `wasm32-unknown-unknown` target?

The first unknown is the system you are compiling on, and the second is the system you are targeting.

“Compile on almost any machine, run on almost any machine.”

Additional information [is available in the Github issue for WASM-bindgen](https://github.com/rustwasm/wasm-bindgen/issues/979).

### Why does the output show "@unknown" instead of "@type" and the decoding failed only showing "@str" and "@bytes"

Check to make sure the module's output type matches the protobuf definition. In some cases, the renamed protobuf package isn't updated in the `substreams.yaml` manifest file's `module.output.type` field, creating an incompatibility.
