---
description: StreamingFast Substreams conceptual diagram
---

# Conceptual Diagram

### Substreams Conceptual Diagram

<img src="../.gitbook/assets/substreams.excalidraw (1).svg" alt="StreamingFast Substreams high-level conceptual diagram" class="gitbook-drawing">

Substreams has two perspectives as illustrated in the high-level visual diagram seen below. One perspective is the architecture of and Substreams engine itself. The other perspective is from that of an end-user developer. &#x20;

Essentially the developer of an end-user application will design and create a data refinement strategy.&#x20;

The Substreams engine will use the data refinement strategy to isolate a very specific data set. Substreams receives data from [StreamingFast Firehose](https://firehose.streamingfast.io/) in the form of streams.&#x20;

The streamed data is passed from Firehose through Substreams, then refined, and finally routed to wherever the developer desires, from relational databases to subgraphs.
