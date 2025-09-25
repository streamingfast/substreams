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

## How It Works

The foundational store processes ERC20 metadata through two mechanisms:

1. **MetadataInitialize Events**: Direct extraction from event data
2. **MetadataChanges Events**: Batch RPC calls to ensure accuracy

Each token address becomes a key, with the corresponding `TokenMetadata` protobuf message as the value.

### Substreams Manifest Configuration

**Producer Module** (creates foundational store entries):
```yaml
specVersion: v0.1.0
package:
  name: evm_token_metadata_foundational_store
  version: v0.1.0

imports:
  erc20_metadata: https://github.com/pinax-network/substreams-evm-tokens/releases/download/erc20-metadata-v0.2.1/evm-erc20-metadata-v0.2.1.spkg

protobuf:
  files:
    - foundational-store.proto
    - token-metadata.proto

modules:
  - name: metadata_to_foundational_store
    kind: map
    inputs:
      - map: erc20_metadata:map_events
    output:
      type: proto:foundational_store.v1.Entries
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

## Integration with Transfer Tracking

The metadata store is designed to work with transfer tracking modules. See the [substreams-erc20-token-metadata](https://github.com/streamingfast/substreams-erc20-token-transfers-with-metadata) example that enriches ERC20 transfers with token metadata by querying this foundational store.
## Related Resources

- [Foundational Stores Overview](../foundational-stores.md)
- [Token Metadata Foundational Store (GitHub)](https://github.com/streamingfast/token-metadata-foundational-store)
- [Pinax EVM Tokens](https://github.com/pinax-network/substreams-evm-tokens) - Base metadata extraction
