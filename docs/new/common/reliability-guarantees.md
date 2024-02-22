
## Reliability Guarantees

When you consume a Substreams package through the CLI (or through any of the different sinks available), you are establishing a gRPC connection with the Substreams provider (i.e. StreamingFast, Pinax...), which streams the data of every block back to your sink.

### The Response Format
The response returned by the provider is a [Protobuf object](https://github.com/streamingfast/substreams/blob/831093480ab6bf6970e41f74ea9bc0b04410a028/proto/sf/substreams/rpc/v2/service.proto#L53), which contains the blockchain data plus other relevant information:

```protobuf
message Response {
  oneof message {
    SessionInit session = 1;
    ModulesProgress progress = 2;
    BlockScopedData block_scoped_data = 3;
    BlockUndoSignal block_undo_signal = 4;
    Error fatal_error = 5;

    InitialSnapshotData debug_snapshot_data = 10;
    InitialSnapshotComplete debug_snapshot_complete = 11;
  }
}
```

### Data & Cursor

One of the most important fields of the response is the `BlockScopedData` object, which contains the actual data of the blockchain, along with other useful fields. Specifically, The `output` field holds the binary data emitted by the Substreams.

```protobuf
message BlockScopedData {
  MapModuleOutput output = 1;
  sf.substreams.v1.Clock clock = 2;
  string cursor = 3;

  uint64 final_block_height = 4;

  repeated MapModuleOutput debug_map_outputs = 10;
  repeated StoreModuleOutput debug_store_outputs = 11;
}
```

In a connection, errors might occur; any of the two parties involved may get disconnected because of a network issue.
In theses cases, it is essential to have a mechanism that allows you to consume the data exactly where you left it before the disconnection. This mechanism is usually called a **cursor**. Essentially, a cursor points to the latest piece of data consumed by the user.

In Substreams, the `cursor` field of the response indicates the latest block consumed by the user. The user **must** persist the cursor, so that in the case of a disconnection, the Substreams provider can start streaming data from the latest consumed block.

For example, the SQL sink establishes a gRPC connection with the Substreams provider, and for every block consumed, it persists the number of the block in a table. If a disconnection occurs, the SQL sink establishes a new connection and starts consuming from the latest persisted block. That's why it is very important to persist the cursor!

### Forks

Forks are really common in blockchain. Essentially, a fork occurs when the path of the blockchain diverges (i.e. there are two or more different paths available because different the nodes involved do not agree on the correct path).

The `BlockUndoSignal` object of the response is used to keep track of forks. In Substreams, you are reading real-time data, so if a fork occurs, you may read blocks from the incorrect path. When the blockchain resolves the fork and eventually chooses a path, you will have to _unread_ all the incorrect blocks (i.e. discard all the blocks belonging to the incorrect path of the fork). The `BlockUndoSignal` contains the latest valid block of the blockchain and a cursor:

```protobuf
message BlockUndoSignal {
  sf.substreams.v1.BlockRef last_valid_block = 1;
  string last_valid_cursor = 2;
}
```

{% hint style="info" %}
If you commit cursors in the BlockUndoSignals, you don’t need to mind about disconnections amid forks. It will bring you back exactly where you left off, even if it was mid-ways through a fork.
{% endhint %}
