---
description: ERC20 Token Metadata Foundational Store
---

# ERC20 Token Metadata Foundational Store

A specialized foundational store for tracking ERC20 token metadata on Ethereum and EVM-compatible chains. This store focuses specifically on metadata extraction and serving, working in conjunction with separate modules for transfer tracking.

## Overview

The ERC20 Token Metadata foundational store provides efficient storage and retrieval of:

- **Token Metadata**: Name, symbol, and decimals for ERC20 tokens
- **Metadata Events**: Initialization and change events for token metadata
- **RPC-Enhanced Data**: Complete metadata fetched via batch RPC calls for accuracy

## Data Model

### Key Structure

The store uses token contract addresses as keys:

```
{token_contract_address} (bytes)
```

### Value Schema

**TokenMetadata** (type.googleapis.com/evm.token.metadata.v1.TokenMetadata)
```protobuf
syntax = "proto3";

package evm.token.metadata.v1;

message TokenMetadata {
  bytes  address  = 1;
  string name     = 2;
  string symbol   = 3;
  int32  decimals = 4;
}
```

## Implementation details

The foundational store processes ERC20 metadata through two mechanisms:

1. **MetadataInitialize Events**: Direct extraction from event data
2. **MetadataChanges Events**: Batch RPC calls to ensure accuracy

Each token address becomes a key, with the corresponding `TokenMetadata` protobuf message as the value.

## Rust Implementation

### Creating Foundational Store Entries

```rust
#[substreams::handlers::map]
fn metadata_to_foundational_store(
    metadata_events: erc20_metadata::Events,
) -> Result<Entries, Error> {
    let mut entries: Vec<Entry> = Vec::new();

    for event in metadata_events.events {
        let token_metadata = TokenMetadata {
            address: event.token_address.clone(),
            name: event.name.clone(),
            symbol: event.symbol.clone(),
            decimals: event.decimals,
        };

        let mut buf = Vec::new();
        Message::encode(&token_metadata, &mut buf).unwrap();

        let any = Any {
            type_url: "type.googleapis.com/evm.token.metadata.v1.TokenMetadata".to_string(),
            value: buf,
        };

        let entry = Entry {
            key: init.address,
            value: Some(any),
        };

        entries.push(entry);

    Ok(Entries { entries })
}
```

> **Complete Example**: See the [map_tokens_transfers](https://github.com/streamingfast/substreams-erc20-token-transfers-with-metadata/blob/main/src/lib.rs#L23) implementation.

### Consuming Foundational Store Data

```rust
#[substreams::handlers::map]
fn map_tokens_transfers(
    block: eth::Block,
    token_metadata_store: FoundationalStore,
) -> Result<TokenTransfers, Error> {
    // ... extract transfers from block

    let response = token_metadata_store.get(&token_address);
    if response.response == ResponseCode::Found as i32 {
        let metadata = TokenMetadata::decode(response.value.unwrap().value.as_slice())?;
        // Use metadata.name, metadata.symbol, metadata.decimals to enrich transfer
    }

    // For multiple tokens: 
    let response = token_metadata_store.get_all(&token_addresses)
    // ...
}
```

> **Complete Example**: See the [metadata_to_foundational_store](https://github.com/Data-Nexus-Web3/token-metadata-foundational-store/blob/main/src/lib.rs) implementation.

### Substreams Manifest Configuration

**Producer Module** (creates foundational store entries):
```yaml
specVersion: v0.1.0
package:
  name: evm_token_metadata_foundational_store
  version: v0.1.0

imports:
  erc20_metadata: https://github.com/pinax-network/substreams-evm-tokens/releases/download/erc20-metadata-v0.2.1/evm-erc20-metadata-v0.2.1.spkg

modules:
  - name: metadata_to_foundational_store
    kind: map
    inputs:
      - map: erc20_metadata:map_events
    output:
      type: proto:sf.substreams.foundational_store.v1.Entries
```

**Consumer Module** (uses foundational store as input):
```yaml
specVersion: v0.1.0
package:
  name: erc20_token_transfers_with_metadata
  version: v0.1.0

imports:
  token_metadata_store: https://github.com/Data-Nexus-Web3/token-metadata-foundational-store/releases/download/v0.1.0/evm-token-metadata-foundational-store-v0.1.0.spkg

modules:
  - name: map_tokens_transfers
    kind: map
    inputs:
      - source: sf.ethereum.type.v2.Block
      - foundational-store: erc20-token-metadata@v0.1.0
    output:
      type: proto:erc20.metadata.v1.TokenTransfers
```

## Related Resources

- [Foundational Stores Overview](../foundational-stores.md)
- [Token Metadata Foundational Store (GitHub)](https://github.com/streamingfast/token-metadata-foundational-store)
- [Pinax EVM Tokens](https://github.com/pinax-network/substreams-evm-tokens) - Base metadata extraction
