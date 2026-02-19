---
description: Substreams RPC protocol versions and performance optimizations
---

# RPC Protocol Reference

Substreams uses gRPC for client-server communication. This document describes the available protocol versions and their performance characteristics.

## Protocol Versions

| Version | Service | Description |
|---------|---------|-------------|
| V2 | `sf.substreams.rpc.v2.Stream/Blocks` | Original protocol, sends modules graph |
| V3 | `sf.substreams.rpc.v3.Stream/Blocks` | Sends full package (spkg) with params |
| V4 | `sf.substreams.rpc.v4.Stream/Blocks` | Batched responses with `BlockScopedDatas` |

### V4 Protocol (Recommended)

V4 is the default protocol starting from v1.18.0. It introduces `BlockScopedDatas`, which batches multiple `BlockScopedData` messages into a single response. This reduces:

- **gRPC round-trips**: Fewer messages means less protocol overhead
- **Message framing cost**: Single frame for multiple blocks
- **Network latency impact**: Particularly beneficial during backfill

The batching is transparent to sink implementations - the client library unpacks `BlockScopedDatas` and delivers individual `BlockScopedData` messages to handlers.

### Protocol Fallback

Clients automatically negotiate the best available protocol:

1. Client attempts V4 connection
2. If server returns `Unimplemented`, client falls back to V3
3. If V3 is also unavailable, client falls back to V2

This ensures compatibility with older servers without configuration changes.

## Compression

### S2 Compression (Default)

S2 is the default compression algorithm, replacing gzip. S2 is part of the Snappy family and provides:

- **~3-5x faster** compression/decompression than gzip
- **Comparable compression ratios** to gzip level 1-2
- **Lower CPU usage** on both client and server
- **Better suited for streaming** workloads

The client requests S2 compression by default. If the server doesn't support S2, standard gzip is used automatically.

### Supported Compression Algorithms

| Algorithm | Name | Notes |
|-----------|------|-------|
| S2 | `s2` | Default, fastest |
| Gzip | `gzip` | Legacy, widely supported |
| LZ4 | `lz4` | Fast, moderate compression |
| Zstd | `zstd` | High compression ratio |

## Connect vs gRPC Protocol Selection

The server supports both Connect RPC and pure gRPC protocols. Starting from v1.18.0, the server efficiently routes requests based on the `Content-Type` header:

| Content-Type | Protocol | Handler |
|--------------|----------|---------|
| `application/grpc`, `application/grpc+proto` | gRPC | Native gRPC handler |
| `application/connect+proto`, `application/json` | Connect | Connect RPC handler |

This routing improves performance by ~15% for pure gRPC clients, which previously had all requests processed through the Connect RPC layer.

{% hint style="info" %}
**Performance tip**: For maximum throughput, use pure gRPC clients when possible. The official Go sink library and Rust client are gRPC-first by default.
{% endhint %}

## VTProtobuf Serialization

Both client and server use [vtprotobuf](https://github.com/planetscale/vtprotobuf) for protobuf marshaling when available. Benefits include:

- **~2-3x faster** serialization/deserialization
- **Reduced memory allocations**
- **Zero-copy unmarshaling** where possible

VTProtobuf is transparent - messages without vtproto support fall back to standard protobuf automatically.

## CLI Usage

### Force Protocol Version

```bash
# Use V4 (default, with batching)
substreams run ... --protocol-version 4

# Use V3 (single-message responses, full package)
substreams run ... --protocol-version 3

# Use V2 (legacy, modules graph only)
substreams run ... --protocol-version 2
```

## Server Configuration

### Environment Variables

| Variable | Description | Default |
|----------|-------------|---------|
| `MESSAGE_BUFFER_MAX_DATA_SIZE` | Max data size (bytes) before flushing a `BlockScopedDatas` batch | 10485760 (10MB) |
| `GRPC_SIZE_LOGGER_MESSAGE_LIMIT` | Enable gRPC message size logging for debugging (set to message count threshold) | Disabled |

### Tier1 Configuration

The `OutputBufferSize` configuration controls how many blocks are batched before sending a `BlockScopedDatas` response:

```go
tier1Config := &app.Tier1Config{
    // ... other config
    OutputBufferSize: 100, // Batch up to 100 blocks
}
```

## Response Messages

### V4 Response Structure

```protobuf
message Response {
  oneof message {
    SessionInit session = 1;
    ModulesProgress progress = 2;
    BlockScopedDatas block_scoped_datas = 3;  // Batched block data
    BlockUndoSignal block_undo_signal = 4;
    Error fatal_error = 5;
    // Debug messages...
  }
}

message BlockScopedDatas {
  repeated BlockScopedData items = 1;
}
```

### V2/V3 Response Structure

```protobuf
message Response {
  oneof message {
    SessionInit session = 1;
    ModulesProgress progress = 2;
    BlockScopedData block_scoped_data = 3;  // Single block data
    BlockUndoSignal block_undo_signal = 4;
    Error fatal_error = 5;
    // Debug messages...
  }
}
```

## Performance Considerations

### When V4 Batching Helps Most

- **Historical backfill**: Processing many blocks sequentially benefits from reduced per-message overhead
- **High-throughput chains**: Chains with fast block times produce more messages per second
- **Network-constrained environments**: Fewer round-trips reduce latency impact

### When Batching Has Less Impact

- **Live streaming at chain head**: Single blocks arrive as produced, batching provides minimal benefit
- **Very large module outputs**: If individual blocks produce large outputs, batching may be limited by `MESSAGE_BUFFER_MAX_DATA_SIZE`

## Compatibility Matrix

| Client Version | Server V2 | Server V3 | Server V4 |
|---------------|-----------|-----------|-----------|
| v1.18.0+ | Yes (fallback) | Yes (fallback) | Yes (native) |
| v1.17.x | Yes | Yes | No |
| v1.16.x | Yes | No | No |
