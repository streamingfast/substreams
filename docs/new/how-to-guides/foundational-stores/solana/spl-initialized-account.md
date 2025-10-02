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
...
use pb::sf::substreams::solana::v1::Transactions as SolanaTransactions;

#[substreams::handlers::map]
fn map_spl_instructions(
    transactions: SolanaTransactions,
    account_owner_store: FoundationalStore,
) -> Result<SplInstructions, Error> {
    // ... extract transfer instructions from transactions

    // Collect account addresses that need owner lookup
    let account_keys: Vec<Vec<u8>> = transfer_accounts.iter()
        .map(|addr| bs58::decode(addr).into_vec().unwrap())
        .collect();

    // Batch query foundational store for account owners
    let response = account_owner_store.get_all(&account_keys);

    // Process responses and decode account owner data
    for entry in response.entries {
        if entry.response.unwrap().response == ResponseCode::Found as i32 {
            let account_owner = AccountOwner::decode(
                entry.response.unwrap().value.unwrap().value.as_slice()
            )?;
            let owner_address = bs58::encode(&account_owner.owner).into_string();
            // Use owner_address to enrich transfer data
        }
    }
    // ...
}
```

**Consumer Module** (uses foundational store as input):
```yaml
specVersion: v0.1.0
package:
  name: solana-spl-all-tokens
  version: v0.1.0

imports:
    solana_common: solana-common@v0.3.0
    spl_initialized_account: spl-initialized-account@v0.1.2

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
#[substreams::handlers::map]
fn map_spl_initialized_account(
    transactions: SolanaTransactions,
) -> Result<Entries, Error> {
    let mut entries: Vec<Entry> = Vec::new();

    for confirmed_trx in successful_transactions(transactions) {
        for instruction in confirmed_trx.walk_instructions() {
            if let Ok(token_instruction) = TokenInstruction::unpack(&instruction.data()) {
                match token_instruction {
                    TokenInstruction::InitializeAccount {} => {
                        let account_owner = AccountOwner {
                            mint_address: instruction.accounts()[1].clone(),
                            owner: instruction.accounts()[2].clone(),
                        };

                        let mut buf = Vec::new();
                        account_owner.encode(&mut buf)?;

                        let entry = Entry {
                            key: instruction.accounts()[0].clone(),
                            value: Some(Any {
                                type_url: "type.googleapis.com/sf.substreams.solana.spl.v1.AccountOwner".to_string(),
                                value: buf,
                            }),
                        };
                        entries.push(entry);
                    }
                    TokenInstruction::InitializeAccount2 { owner } => {
                        // Handle embedded owner in instruction data
                    }
                    _ => {}
                }
            }
        }
    }

    Ok(Entries { entries })
}
```

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

> **Complete Example**: See the [map_spl_initialized_account](https://github.com/streamingfast/substreams-foundational-modules/blob/develop/solana/spl-initialized-account/src/lib.rs#L23) implementation.

## Related Resources

- [Foundational Stores Overview](../foundational-stores.md)
- [Introduction to Foundational Stores](../../../tutorials/intro-to-foundational-stores.md)
- [SPL Initialized Account (GitHub)](https://github.com/streamingfast/substreams-foundational-modules/tree/develop/solana/spl-initialized-account)
- [Substreams SPL All Tokens](https://github.com/streamingfast/substreams-spl-all-tokens) - Consumer example
