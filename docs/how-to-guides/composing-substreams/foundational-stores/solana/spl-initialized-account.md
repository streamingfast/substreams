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

SPL token transfer instructions on Solana only contain account addresses, not the wallet owners. To determine who actually sent/received tokens, you need to resolve account ownership. This foundational store provides that critical mapping.

## Consuming Account Owner Data

```rust
use substreams::store::FoundationalStore;
use std::collections::HashSet;

#[substreams::handlers::map]
fn map_spl_instructions(
    params: String,
    transactions: SolanaTransactions,
    foundational_store: FoundationalStore,
) -> Result<SplInstructions, Error> {
    // ... extract transfer instructions from transactions

    // Collect account addresses that need owner lookup
    let mut accounts_to_lookup = HashSet::<String>::new();
    accounts_to_lookup.insert(transfer.from.clone());
    accounts_to_lookup.insert(transfer.to.clone());

    // Convert addresses to bytes and query store
    let account_bytes: Vec<Vec<u8>> = accounts_to_lookup
        .iter()
        .filter_map(|addr| bs58::decode(addr).into_vec().ok())
        .collect();

    let resp = foundational_store.get(&account_bytes);

    // Process responses and decode owner data
    for queried_entry in resp.entries {
        if queried_entry.code != ResponseCode::Found as i32 {
            continue;
        }
        let Some(entry) = &queried_entry.entry else { continue; };
        let Some(value) = &entry.value else { continue; };

        let Ok(account_owner) = AccountOwner::decode(value.value.as_slice()) else {
            continue;
        };

        // Use owner data to enrich transfers
        let owner_b58 = bs58::encode(&account_owner.owner).into_string();
        transfer.from_owner = owner_b58;
    }

    Ok(SplInstructions { instructions })
}
```

**Consumer Module** (uses foundational store as input):
```yaml
specVersion: v0.1.0
package:
  name: solana-spl-token
  version: v0.2.0

imports:
  solana_common: solana-common@v0.3.0
  spl_initialized_account: spl-initialized-account@v0.2.0

modules:
  - name: map_spl_instructions
    kind: map
    initialBlock: 158569587
    inputs:
      - params: string
      - map: solana_common:transactions_by_programid_and_account_without_votes
      - foundational-store: spl-initialized-account@v0.1.2
    output:
      type: proto:sf.solana.spl.v1.type.SplInstructions

params:
  map_spl_instructions: "spl_token_address=EPjFWdd5AufqSSqeM2qN1xzybapC8G4wEGGkZwyTDt1v|spl_token_decimal=6"
  solana_common:transactions_by_programid_and_account_without_votes: "program:TokenkegQfeZyiNwAJbNbGKPFXCWuBvf9Ss623VQ5DA || program:TokenzQdBNbLqP5VEhdkAS6EPFLC1PHnBqCXEpPxuEb"
```

> **Complete Example**: See the [map_spl_instructions](https://github.com/streamingfast/substreams-spl-token/blob/main/src/lib.rs#L49) implementation.

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

## Implementation details

The foundational store processes SPL token account initialization instructions to extract account-to-owner mappings. It tracks three instruction types:

1. **`InitializeAccount`** - Basic account initialization with separate owner account
2. **`InitializeAccount2`** - Account initialization with embedded owner in instruction data
3. **`InitializeAccount3`** - Newer variant of account initialization with embedded owner

Each SPL token account address becomes a key, with the corresponding `AccountOwner` protobuf message containing mint and owner information as the value.

## Implementation

### Creating Foundational Store Entries

```rust
use prost::Message;

#[substreams::handlers::map]
pub fn map_spl_initialized_account(
    transactions: SolanaTransactions
) -> Result<SinkEntries, Error> {
    let mut entries: Vec<Entry> = vec![];

    for transaction in transactions.transactions {
        if !transaction.is_successful() {
            continue;
        }

        for instruction in transaction.walk_instructions() {
            // Decode SPL token instruction
            let token_instruction = TokenInstruction::unpack(instruction.data().as_slice())?;

            match token_instruction {
                TokenInstruction::InitializeAccount {} => {
                    let account_owner = AccountOwner {
                        mint_address: instruction.accounts()[1].clone(),
                        owner: instruction.accounts()[2].clone(),
                    };

                    let mut buf = Vec::new();
                    prost::Message::encode(&account_owner, &mut buf).unwrap();

                    entries.push(Entry {
                        key: Some(Key {
                            bytes: instruction.accounts()[0].to_vec(),
                        }),
                        value: Some(Any {
                            type_url: "type.googleapis.com/sf.substreams.solana.spl.v1.AccountOwner".to_string(),
                            value: buf,
                        }),
                    });
                }
                TokenInstruction::InitializeAccount2 { owner } |
                TokenInstruction::InitializeAccount3 { owner } => {
                    // Handle embedded owner in instruction data
                    // ... similar to InitializeAccount
                }
                _ => {}
            }
        }
    }

    Ok(SinkEntries {
        entries,
        if_not_exist: true,
    })
}
```

### Substreams Manifest Configuration

**Producer Module** (creates foundational store entries):
```yaml
specVersion: v0.1.0
package:
  name: spl-initialized-account
  version: v0.2.1

network: solana

imports:
  solana_common: solana-common@v0.3.0

modules:
  - name: map_spl_initialized_account
    kind: map
    initialBlock: 31310775
    inputs:
      - params: string
      - map: solana_common:transactions_by_programid_without_votes
    output:
      type: proto:sf.substreams.foundational_store.model.v2.SinkEntries

params:
  solana_common:transactions_by_programid_without_votes: "program:TokenkegQfeZyiNwAJbNbGKPFXCWuBvf9Ss623VQ5DA || program:TokenzQdBNbLqP5VEhdkAS6EPFLC1PHnBqCXEpPxuEb"
```

> **Complete Example**: See the [map_spl_initialized_account](https://github.com/streamingfast/substreams-foundational-modules/blob/develop/solana/spl-initialized-account/src/lib.rs#L23) implementation.

## Related Resources

- [Hosting a Foundational Store](https://docs.substreams.dev/reference-material/foundational-store-reference/hosting-foundational-stores)
- [Consuming a Foundational Store](https://docs.substreams.dev/tutorials/consuming-foundational-store)
- [Foundational Stores Architecture](../../../../references/foundational-store-reference.md)
