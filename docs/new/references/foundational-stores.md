---
description: Foundational Stores Overview
---

# Foundational Stores

A high-performance, multi-backend key-value storage system designed for Substreams ingestion and serving within the StreamingFast ecosystem. The foundational store provides a unified interface to persist and query time-series blockchain data with fork-awareness and efficient batch processing.

## What are Foundational Stores?

Foundational stores are persistent storage systems that serve as the foundation for complex blockchain data applications. They provide a unified interface for ingesting, storing, and serving time-series blockchain data with fork-awareness capabilities.

- **Block-level versioning**: Every piece of data is tagged with the block number where it originated
- **Fork-aware operations**: Automatic handling of blockchain reorganizations with rollback capabilities
- **Real-time ingestion**: Continuous processing of streaming Substreams output
- **High-performance serving**: Optimized gRPC API for fast data retrieval

## Architecture

Foundational stores consist of three main components working together:

### Sink Component

The **Sink** handles streaming data ingestion from Substreams:

- **Continuous Processing**: Consumes Substreams output in real-time
- **Batch Management**: Groups data into efficient batches for storage
- **Cursor Management**: Maintains resumption state for reliable streaming
- **Fork Detection**: Processes `BlockUndoSignal` messages for reorganizations

### Store Component

The **Store** provides a unified interface across multiple storage backends:

- **ForkAware Layer**: In-memory cache with block-level versioning
- **Backend Abstraction**: Support for Badger (embedded) and PostgreSQL
- **Flush Operations**: Persists finalized data below Last Irreversible Block (LIB)
- **Eviction Handling**: Removes reorganized data during fork events

### Server Component

The **Server** exposes a high-performance gRPC API:

- **Get/GetAll Operations**: Retrieve single or multiple keys efficiently
- **Block Validation**: Ensures requested blocks have been processed
- **Response Codes**: Clear status indicators (FOUND, NOT_FOUND, NOT_FOUND_BLOCK_NOT_REACHED)

## Chain-Specific Foundational Stores

Foundational stores are available for various blockchain ecosystems, each optimized for specific data patterns and use cases:

### Ethereum
- [ERC20 Token Metadata](foundational-stores/ethereum/erc20-token-metadata.md) - Store and serve ERC20 token metadata and balances

### Solana
- [SPL Initialized Account](foundational-stores/solana/spl-initialized-account.md) - Track SPL token account initialization and ownership
