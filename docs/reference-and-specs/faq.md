---
description: StreamingFast Substreams frequently asked questions
---

# FAQ

### **What is Substreams?**

Substreams is an exceptionally powerful processing engine capable of consuming streams of rich blockchain data. Substreams refines and shapes the data for painless digestion by end-user applications, such as decentralized exchanges.

### **Do I need Firehose to use Substreams?**

Developers do not need a dedicated installation of Firehose to work with Substreams. StreamingFast provides a public Firehose endpoint made available to developers.

### **Can I use Substreams in my subgraph?**

Yes, Substreams compliments and extend the capabilities and functionalities of subgraphs. Additional information is available in the Substreams documentation for working with Graph Node and subgraphs.

[https://substreams.streamingfast.io/reference-and-specs/graph-node-setup](https://substreams.streamingfast.io/reference-and-specs/graph-node-setup)

### **Can I use Substreams for production deployments?**

Substreams is provided as a developer preview at this point in time. StreamingFast is working to enable a fully production-ready solution in the very near future.

### **What is the Substreams CLI?**

The Substreams command line interface, or CLI, is the central access point that developers use to interact with the Substreams engine. The Substreams CLI provides a range of features, commands, and flags. Additional information for working with the CLI is available in the Substreams documentation.

[https://substreams.streamingfast.io/reference-and-specs/using-the-cli](https://substreams.streamingfast.io/reference-and-specs/using-the-cli)

### **How do I get a Substreams authentication token?**

Authentication tokens are required to work with Substreams and connect to the public Firehose endpoint. Full instructions for obtaining a StreamingFast authentication token are available in the Substreams documentation.

[https://substreams.streamingfast.io/reference-and-specs/authentication](https://substreams.streamingfast.io/reference-and-specs/authentication)

### **My Substreams authentication token isn’t working, what do I do?**

The StreamingFast team is available in Discord to assist with problems related to obtaining or using authentication tokens.&#x20;

[https://discord.gg/Ugc7KtkA](https://discord.gg/Ugc7KtkA)

The Substreams documentation also provides general instructions surrounding authentication tokens.

[https://substreams.streamingfast.io/reference-and-specs/authentication](https://substreams.streamingfast.io/reference-and-specs/authentication)

### **How do I make a Substream?**

Developers can create their own Substreams implementations in a variety of ways. StreamingFast provides the Substreams Playground that has examples to use as a reference and starting point.

[https://github.com/streamingfast/substreams-playground](https://github.com/streamingfast/substreams-playground)

The Substreams documentation also provides a Developer Guide to assist with understanding and working with Substreams.

[https://substreams.streamingfast.io/developer-guide/overview](https://substreams.streamingfast.io/developer-guide/overview)

### **What is Substreams for?**

Substreams works in conjunction with StreamingFast Firehose to enable extremely fast access and processing capabilities of blockchain data. Substreams is used for transforming rich blockchain data and exposing it to the needs of application developers.

Additional information is available on the What is Substreams page in the documentation.

[https://substreams.streamingfast.io/concept-and-fundamentals/definition](https://substreams.streamingfast.io/concept-and-fundamentals/definition)

### **Is Substreams free?**

Yes, Substreams is an open-source project and available to the public for free.

### **How would a developer access the information returned from a call to Substreams from a web-based UI?**

Right now, it’s only command line. Substreams is not meant to be piped to a web UI, it’s a data transformation layer. Eventually, Substreams will reach Subgraphs (as a data source), and at that point, Subgraphs makes a GraphQL available for web consumption. Also, other sinks might expose APIs for web browsers. It’s just not Substreams’ responsibility.
