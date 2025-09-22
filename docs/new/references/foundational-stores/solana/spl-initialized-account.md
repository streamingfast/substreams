---
description: SPL Initialized Account Foundational Store
---

# SPL Initialized Account Foundational Store

A specialized foundational store for tracking SPL token account initializations on Solana. This store provides the essential account-to-owner mappings needed to resolve SPL token transfers, since transfer instructions only contain account addresses without owner information.

## Overview

The SPL Initialized Account foundational store provides efficient storage and retrieval of:

- **Account-Owner Mappings**: Relationship between SPL token accounts and their owners
- **Mint Associations**: Which token mint each account is associated with
- **Initialization Events**: Tracking of newly created SPL token accounts

## Problem Solved

SPL token transfer instructions on Solana only contain account addresses, not the wallet owners. To determine who actually sent/received tokens, you need to resolve account ownership. This foundational store provides that critical mapping.

## Data Model

### Key Structure

The store uses SPL token account addresses as keys:

```
{spl_token_account_address} (bytes)
```

### Value Schema

**AccountOwner** (type.googleapis.com/sf.substreams.solana.spl.v1.AccountOwner)
```protobuf
syntax = "proto3";

package sf.substreams.solana.spl.v1;

// Represents ownership relationship between a mint and its owner
// Both in raw bytes
message AccountOwner {
  bytes mint_address = 2;
  bytes owner = 3;
}
```

## How It Works

The foundational store processes SPL token account initialization instructions to extract account-to-owner mappings. It tracks three instruction types:

1. **`InitializeAccount`** - Basic account initialization with separate owner account
2. **`InitializeAccount2`** - Account initialization with embedded owner in instruction data
3. **`InitializeAccount3`** - Newer variant of account initialization with embedded owner

Each SPL token account address becomes a key, with the corresponding `AccountOwner` protobuf message containing mint and owner information as the value.

### Substreams Manifest Configuration

**Producer Module** (creates foundational store entries):
```yaml
specVersion: v0.1.0
package:
  name: spl-initialized-account
  version: v0.1.2

network: solana

imports:
  solana_common: solana-common@v0.3.0

protobuf:
  files:
    - sf/substreams/solana/spl/v1/initialized_account.proto
  descriptorSets:
    - module: buf.build/streamingfast/substreams-foundational-store

modules:
  - name: map_spl_initialized_account
    kind: map
    initialBlock: 31310775
    inputs:
      - params: string
      - map: solana_common:transactions_by_programid_without_votes
    output:
      type: proto:sf.substreams.foundational_store.v1.Entries

params:
  solana_common:transactions_by_programid_without_votes: "program:TokenkegQfeZyiNwAJbNbGKPFXCWuBvf9Ss623VQ5DA || program:TokenzQdBNbLqP5VEhdkAS6EPFLC1PHnBqCXEpPxuEb"
```

**Consumer Module** (uses foundational store as input):
```yaml
specVersion: v0.1.0
package:
  name: solana-spl-all-tokens
  version: v0.1.0

imports:
  solana_common: solana-common@v0.3.0
  spl_owner_foundational: ../substreams-foundational-modules/solana/spl-initialized-account/spl-initialized-account-v0.1.2.spkg

modules:
  - name: map_spl_instructions
    kind: map
    initialBlock: 31310775
    inputs:
      - map: solana_common:transactions_by_programid_without_votes
      - foundational-store: spl-initialized-account@v0.1.2
    output:
      type: proto:sf.solana.spl.v1.type.SplInstructions

params:
  solana_common:transactions_by_programid_without_votes: "program:TokenkegQfeZyiNwAJbNbGKPFXCWuBvf9Ss623VQ5DA || program:TokenzQdBNbLqP5VEhdkAS6EPFLC1PHnBqCXEpPxuEb"
```



## Integration with Transfer Tracking

The account ownership store is essential for SPL token transfer modules. See the [substreams-spl-all-tokens](https://github.com/streamingfast/substreams-spl-all-tokens) example that enriches SPL token transfers with wallet owner information by querying this foundational store.


## Related Resources

- [Foundational Stores Overview](../foundational-stores.md)
- [SPL Initialized Account (GitHub)](https://github.com/streamingfast/substreams-foundational-modules/tree/develop/solana/spl-initialized-account)
- [Substreams SPL All Tokens](https://github.com/streamingfast/substreams-spl-all-tokens) - Consumer example