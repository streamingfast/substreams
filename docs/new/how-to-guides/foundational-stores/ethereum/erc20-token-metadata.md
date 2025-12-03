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

> **Note**: This foundational store is currently deployed on **Ethereum Mainnet only** for testing purposes. For deployments on other networks, please reach out on [Discord](https://discord.com/invite/jZwqxJAvRs).

## Consuming Foundational Store Data

```rust
use substreams::store::FoundationalStore;
use substreams_ethereum::pb::eth::v2::Block;

#[substreams::handlers::map]
fn map_tokens_transfers(
    block: Block,
    foundational_store: FoundationalStore,
) -> Result<TokenTransfers, Error> {
    // ... extract transfers from block

    // Collect token addresses that need metadata lookup
    let keys_to_query: Vec<Vec<u8>> = token_addresses_to_resolve.into_iter().collect();
    let resp = foundational_store.get_all(&keys_to_query);

    // Process responses and decode metadata
    let mut metadata_map = std::collections::HashMap::new();
    for entry in resp.entries {
        let code = ResponseCode::try_from(entry.response.as_ref().unwrap().response)?;
        if code != ResponseCode::Found {
            continue;
        }

        if let Ok(token_metadata) = TokenMetadata::decode(entry.response.unwrap().value.unwrap().value.as_slice()) {
            metadata_map.insert(entry.key, token_metadata);
        }
    }

    // Use metadata_map to enrich transfers with name, symbol, decimals
    Ok(TokenTransfers { transfers })
}
```

**Consumer Module** (uses foundational store as input):
```yaml
specVersion: v0.1.0
package:
  name: erc20_token_transfers_with_metadata
  version: v0.2.0

modules:
  - name: map_tokens_transfers
    kind: map
    inputs:
      - source: sf.ethereum.type.v2.Block
      - foundational-store: erc20-token-metadata@v0.1.0
    output:
      type: proto:erc20.metadata.v1.TokenTransfers

network: mainnet
```
> **Complete Example**: See the [map_tokens_transfers](https://github.com/streamingfast/substreams-erc20-token-transfers-with-metadata/blob/main/src/lib.rs#L23) implementation.

## Data Model

### Key Structure

The store uses token contract addresses as keys:

```
{token_contract_address} (bytes)
```

### Value Schema

**TokenMetadata** (type.googleapis.com/sf.substreams.ethereum.erc20.v1.TokenMetadata)
```protobuf
syntax = "proto3";

package sf.substreams.ethereum.erc20.v1;

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

## Implementation

### Creating Foundational Store Entries

```rust
use prost::Message;
use prost_types::Any;

#[substreams::handlers::map]
fn metadata_to_foundational_store(
    events: erc20_metadata::Events,
) -> Result<SinkEntries, Error> {
    let mut entries = Vec::new();

    // For MetadataInitialize events, create TokenMetadata directly
    for init in events.metadata_initialize {
        let token_metadata = TokenMetadata {
            address: init.address.clone(),
            name: init.name.unwrap_or_default(),
            symbol: init.symbol.unwrap_or_default(),
            decimals: init.decimals,
        };

        let mut buf = Vec::new();
        Message::encode(&token_metadata, &mut buf).unwrap();

        entries.push(Entry {
            key: Some(Key {
                bytes: init.address
            }),
            value: Some(Any {
                type_url: "type.googleapis.com/sf.substreams.ethereum.erc20.v1.TokenMetadata".to_string(),
                value: buf,
            }),
        });
    }

    // For MetadataChanges events, fetch full metadata via RPC
    // ... batch RPC calls to get name, symbol, decimals
    // ... create entries with updated metadata

    Ok(SinkEntries {
        entries,
        if_not_exist: false,
    })
}
```

### Substreams Manifest Configuration

**Producer Module** (creates foundational store entries):
```yaml
specVersion: v0.1.0
package:
  name: erc20-token-metadata
  version: v0.2.0

network: mainnet

imports:
  erc20_metadata: https://github.com/pinax-network/substreams-evm-tokens/releases/download/erc20-metadata-v0.2.1/evm-erc20-metadata-v0.2.1.spkg

modules:
  - name: metadata_to_foundational_store
    kind: map
    inputs:
      - map: erc20_metadata:map_events
    output:
      type: proto:sf.substreams.foundational_store.model.v2.SinkEntries
```

> **Complete Example**: See the [metadata_to_foundational_store](https://github.com/streamingfast/substreams-foundational-modules/blob/develop/ethereum/erc20-token-metadata/src/lib.rs#L11) implementation.

## Related Resources

- [Hosting a Foundational Store](https://docs.substreams.dev/tutorials/hosting-foundational-stores)
- [Consuming a Foundational Store](https://docs.substreams.dev/tutorials/consuming-foundational-store)
- [Foundational Stores Architecture](../../../references/foundational-store-reference.md)
