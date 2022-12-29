---
description: StreamingFast Substreams frequently asked questions
---

# FAQ

### is **What is Substreams?**

Substreams is an exceptionally powerful processing engine capable of consuming streams of rich blockchain data. Substreams refines and shapes the data for painless digestion by end-user applications, such as decentralized exchanges.

### **Do I need Firehose to use Substreams?**

Developers do not need a dedicated installation of Firehose to use Substreams. StreamingFast provides a public Firehose endpoint made available to developers.

### **Is it possible to use Substreams in my subgraph?**

Yes, Substreams compliments and extend the capabilities and functionalities of subgraphs. Additional information is available in the Substreams documentation for using graph-node and subgraphs.

[https://substreams.streamingfast.io/reference-and-specs/graph-node-setup](https://substreams.streamingfast.io/reference-and-specs/graph-node-setup)

### **Is it possible to use Substreams for production deployments?**

No, Substreams is provided as a developer preview.

### **What is the Substreams CLI?**

The Substreams command line interface (CLI) is the main tool developers use to use the Substreams engine. The Substreams CLI provides a range of features, commands, and flags. Additional information for using the CLI is available in the Substreams documentation.

[https://substreams.streamingfast.io/reference-and-specs/using-the-cli](https://substreams.streamingfast.io/reference-and-specs/using-the-cli)

### **How do I get a Substreams authentication token?**

Authentication tokens are required to use Substreams and connect to the public Firehose endpoint. Full instructions for obtaining a StreamingFast authentication token are available in the Substreams documentation.

[https://substreams.streamingfast.io/reference-and-specs/authentication](https://substreams.streamingfast.io/reference-and-specs/authentication)

### **My Substreams authentication token isn’t working, what do I do?**

The StreamingFast team is available in Discord to resolve problems related to obtaining or using authentication tokens.&#x20;

[https://discord.gg/Ugc7KtkA](https://discord.gg/Ugc7KtkA)

The Substreams documentation also provides general instructions surrounding authentication tokens.

[https://substreams.streamingfast.io/reference-and-specs/authentication](https://substreams.streamingfast.io/reference-and-specs/authentication)

### **How do I create a Substreams implementation?**

Developers will create their own Substreams implementations in a variety of ways. StreamingFast provides the Substreams Playground containing examples to use as a reference and starting point.

[https://github.com/streamingfast/substreams-playground](https://github.com/streamingfast/substreams-playground)

The Substreams documentation also provides a Developer's Guide to assist in understanding and using Substreams.

[https://substreams.streamingfast.io/developer-guide/overview](https://substreams.streamingfast.io/developer-guide/overview)

### **What is Substreams for?**

Substreams and Firehose work together to index and process blockchain data. Substreams is used for transforming rich blockchain data and exposing it to the needs of application developers.

Additional information is available on the What is Substreams page in the documentation.

[https://substreams.streamingfast.io/concept-and-fundamentals/definition](https://substreams.streamingfast.io/concept-and-fundamentals/definition)

### **Is Substreams free?**

Yes, Substreams is an open source project and is available to the public for free.

### **How does a developer reach the information returned from a call to Substreams from a web-based UI?**

Substreams is not meant to be piped to a web UI, it’s a data transformation layer. Substreams will reach Subgraphs, as a data source, and make GraphQL available for web consumption. Other sinks might expose APIs for web browsers, however, it's not Substreams’ responsibility.

### Is it possible to listen for new blocks using Substreams?

Specifying a stop block value of zero (0), the default enables transparent handoff from historical to real-time blocks.

### **Does StreamingFast have a Discord?**

Yes! Join the StreamingFast Discord!

[https://discord.gg/Ugc7KtkA](https://discord.gg/mYPcRAzeVN)

### **Is StreamingFast on Twitter?**

Yes! Find StreamingFast on their official Twitter account.

[https://twitter.com/streamingfastio](https://twitter.com/streamingfastio)

### **Is StreamingFast on YouTube?**

Yes! Find StreamingFast on their official YouTube account.

[https://www.youtube.com/c/streamingfast](https://www.youtube.com/c/streamingfast)

### **Who is dfuse?**

StreamingFast was originally called dfuse. The company changed its name and is in the process of phasing the old brand out.

### What is Sparkle?

Substreams is the successor of [StreamingFast Sparkle](https://github.com/streamingfast/sparkle). Substreams enables greater composability, and provides similar powers of parallelization. Sparkle is deprecated.

### **Who is StreamingFast?**

StreamingFast is a protocol infrastructure company providing a massively scalable architecture for streaming blockchain data. StreamingFast is one of the core developers working alongside The Graph Foundation.

### Why the `wasm32-unknown-unknown` target?

The first unknown is the system you are compiling on, and the second is the system you are targeting.

“Compile on almost any machine, run on almost any machine.”

Additional information is available in the Github issue for WASM-bindgen:

[https://github.com/rustwasm/wasm-bindgen/issues/979](https://github.com/rustwasm/wasm-bindgen/issues/979)

### Why does the output show "@unknown" instead of "@type" and the decoding failed only showing "@str" and "@bytes"

Check to make sure the module's output type matches the `protobuf` definition. In some cases, the renamed  `protobuf` package isn't updated in the `substreams.yaml` manifest's `module.output.type` field for correct alignment.
