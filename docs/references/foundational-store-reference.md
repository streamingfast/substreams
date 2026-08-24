---
description: Foundational Store architecture, components, and technical reference
---

# Substreams Foundational Store

A high-performance, multi-backend key-value storage system designed for [Substreams](https://github.com/streamingfast/substreams) ingestion and serving within the StreamingFast ecosystem. The foundational store provides a unified interface to persist and query time-series blockchain data with fork-awareness and efficient batch processing.

## StreamingFast Ecosystem Integration

The foundational store operates as a critical component in the StreamingFast data processing pipeline:

- **Tier1 (Substreams Frontend)**: Client-facing gRPC service that handles user requests, manages authentication, and orchestrates work distribution to Tier2 execution engines with foundational store endpoint routing
- **Tier2 (Substreams Execution Engine)**: Computational backend service that executes Substreams WASM modules in parallel across blockchain data segments, handling module execution and state management
- **Foundational Store**: Persistent storage layer serving multiple Substreams modules simultaneously

### Deployment Patterns

- **Many-to-Many Architecture**: Multiple Substreams modules can target the same foundational store
- **Multi-Store Deployments**: Multiple foundational stores can run simultaneously, each serving multiple endpoints
- **Flexible Routing**: Tier1 routes requests via configuration
- **Module Examples**: Custom Substreams modules for any blockchain data processing use case

## Architecture

The foundational store consists of three main components:

- **Sink**: Ingests streaming data from Substreams, handles batching, flushing, and fork reorganizations
- **Store**: Provides a unified interface for multiple storage backends (Badger, PostgreSQL) with ForkAware caching layer
- **Server**: Exposes a gRPC API for data retrieval with high-performance querying and block-aware responses

### Key Features

- **Fork-aware storage**: Handles blockchain reorganizations through ForkAware wrapper with in-memory cache and automatic rollback capabilities
- **Multiple backends**: Support for embedded Badger database and PostgreSQL with unified Store interface
- **Block-level versioning**: Every entry tagged with block number for precise historical queries and LIB-based finality
- **Conditional operations**: IfNotExist flag prevents duplicate insertions and ensures data integrity
- **Streaming ingestion**: Continuous processing of Substreams output with cursor-based resumption
- **High-performance serving**: gRPC API with Get/GetFirst operations and block-reached validation

## Quick Start

### Installation

Build from source:
```bash
git clone https://github.com/streamingfast/substreams-foundational-store
cd substreams-foundational-store
go build -o foundational-store ./cmd/foundational-store
```

See [Hosting a Foundational Store](./foundational-stores/hosting-foundational-stores.md) for complete setup and configuration instructions.

## Storage Backends

### Badger

High-performance embedded key-value store, ideal for single-node deployments:

```bash
--dsn "badger:///path/to/database"
```

### PostgreSQL

Enterprise-grade relational database for distributed deployments:

```bash
--dsn "postgres://user:password@host:port/database?sslmode=require"
```

See [Hosting a Foundational Store](foundational-stores/hosting-foundational-stores.md) for backend-specific configuration and tuning.

## Configuration

The `foundational-store` binary provides the following commands:

```bash
foundational-store [command]

Available Commands:
  completion  Generate the autocompletion script for the specified shell
  get         Get a value from the foundational-store using gRPC
  help        Help about any command
  server      Start the gRPC server
```

See [Hosting a Foundational Store](foundational-stores/hosting-foundational-stores.md) for detailed server configuration options and usage examples.

## Data Model

### Entry Structure

Data is stored as key-value pairs with block-level versioning:

```protobuf
// Current v2 API (recommended)
message Entry {
  Key key = 2;
  google.protobuf.Any value = 4;
}

message Key {
  bytes bytes = 1;
}

message QueriedEntry {
  ResponseCode code = 1;
  Entry entry = 2;
}

message QueriedEntries {
  repeated QueriedEntry entries = 2;
}

// Batch operations with conditional insertion
message SinkEntries {
  repeated Entry entries = 1;
  bool if_not_exist = 2;  // Skip insertion if key already exists
}
```

### API Operations

The Foundational Store provides gRPC APIs for data retrieval with block-aware querying.

See [Consuming a Foundational Store](../tutorials/consuming-foundational-store.md) for detailed API usage, response handling, and code examples.

### Conditional Operations

The store supports conditional insertion with the `if_not_exist` flag for data integrity during ingestion.

See [Hosting a Foundational Store](foundational-stores/hosting-foundational-stores.md) for details on using `SinkEntries` and conditional operations.

**Note**: v1 API is deprecated. Use v2 API for all new implementations.

### API Version History

- **v2** (current): Improved service interface with `Get` and `GetFirst` operations, enhanced data models
- **v1** (deprecated): Legacy interface with separate `Get` and `GetAll` operations, will be removed in a future version

Migration guide: Replace v1 service calls with v2 equivalents. Update message types to use `sf.substreams.foundational_store.model.v2` and `sf.substreams.foundational_store.service.v2`.

## Fork Handling

The foundational store implements sophisticated fork-awareness through a layered architecture:

### ForkAware Store Layer

1. **In-Memory Cache**: Maintains recent entries in memory with block-level versioning
2. **Automatic Eviction**: `EvictUpToBlock()` removes data >= reorganization point during undo signals
3. **LIB-Based Flushing**: `FlushUpToBlock()` persists finalized entries (≤ Last Irreversible Block) to backend
4. **Read Strategy**: Checks cache first, falls back to persistent backend for historical data

### Block Processing Flow

1. **HandleBlockScopedData**: Processes streaming data, updates cache, flushes finalized blocks
2. **HandleBlockUndoSignal**: Triggers eviction on fork detection, maintains data consistency
3. **Cursor Management**: Persistent state tracking with LIB-based cursor history cleanup
4. **Head Block Tracking**: Real-time block progression for client synchronization validation

## Health Checks

Monitor service health through:
- gRPC reflection for service discovery
- Cursor file updates for ingestion progress
- Prometheus `/metrics` endpoint availability

## Documentation

Comprehensive API documentation is available in the proto files:
- `proto/sf/substreams/foundational-store/service/v2/service.proto` - Current gRPC service API
- `proto/sf/substreams/foundational-store/model/v2/model.proto` - Data model definitions

## Related Resources

- [Hosted Services](../how-to-guides/hosted-services/hosted-services.md) - Managed sinks and stores StreamingFast runs for you
- [Remote Feed Hosted Store Guide](../how-to-guides/hosted-services/hosted-store-remote-feed.md) - Write to and read from a StreamingFast-managed hosted store over gRPC, without running your own server
- [Substreams Feed Hosted Store Guide](../how-to-guides/hosted-services/hosted-store-substreams-feed.md) - Have a Substreams package populate a StreamingFast-managed hosted store from the chain
- [Hosting a Foundational Store](foundational-stores/hosting-foundational-stores.md) - Complete guide for setting up and running a Foundational Store server
- [Consuming a Foundational Store](../tutorials/consuming-foundational-store.md) - Guide for querying Foundational Stores in Substreams modules
- [Foundational Store Examples](../how-to-guides/composing-substreams/foundational-stores/foundational-stores.md) - Chain-specific foundational store implementations
- [GitHub Repository](https://github.com/streamingfast/substreams-foundational-store) - Source code and issue tracker
- [Substreams](https://github.com/streamingfast/substreams) - Real-time blockchain data processing
- [Firehose](https://github.com/streamingfast/firehose) - Blockchain data extraction protocol
