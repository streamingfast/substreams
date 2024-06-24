It is possible to the send data the of a Substreams to a subgraph, thus creating a Substreams-powered Subgraph.

There are two ways of making Substreams interact with subgraphs:
- Create a special [graph_out module](./graph-out.md) that emits an [EntityChanges](https://github.com/streamingfast/substreams-sink-entity-changes/blob/develop/proto/sf/substreams/sink/entity/v1/entity.proto#L11) Protobuf.
The subgraph will read the `EntityChanges` object and consume the data.
- Use the [**Substreams triggers**](./triggers.md) to consume Substreams Protobuf directly inside your subgraph.

## What Option To Use
Both are valid options depending on the use case:
- Use the `graph_out` module is if your logic is entirely in the Substreams Rust code. You will create the subgraph entities in the Substreams and the subgraph will only read them. By putting your logic in the Substreams, you will **benefit from parallelism and you will be able to handle a cursor to track the progress of the data**. Read more about it in the [Reliability Guarantees](../../common/reliability-guarantees.md) section. 

- Use the **Substreams triggers** if you want to use Substreams as an extraction layer for the data, but the logic of your application will be in the subgraph AssemblyScript code. By putting all your logic in the subgraph, you will deal with an _easier_ programming language (AssemblyScript).

<figure><img src="../../../.gitbook/assets/consume/service-subgraph.png" width="100%" /></figure>