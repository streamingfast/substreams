It is possible to the send data the of a Substreams to a subgraph, thus creating a Substreams-powered Subgraph.

There are two ways of making Substreams interact with subgraphs:
- Create a special [`graph_out` module](./graph-out.md) that emits an [EntityChanges]() Protobuf.
The subgraph will read the `EntityChanges` object and consume the data.
- Use the [**Substreams triggers**](./triggers.md) to consume Substreams Protobuf inside your subgraph.

<figure><img src="../../../.gitbook/assets/consume/service-subgraph.png" width="100%" /></figure>