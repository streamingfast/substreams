---
description: Retrieving transactions on EVM blockchains
---

In EVM-compatible chains, a Trasanction represents an change in the blockchain, such as ETH transfers or smart contract executions. In Substreams, transactions are abstracted by the [TransactionTrace](https://github.com/streamingfast/firehose-ethereum/blob/develop/proto/sf/ethereum/type/v2/type.proto#L157) Protobuf model. Some of the most relevant fields and methods of the model are:
- `hash` (property): hash of the transaction.
- `from` (property): `from` field of the transaction.
- `to` (property): `to` field of the transaction.
- `input`: (property): `input` field of the transaction.
- `receipt`: (property): [TransactionReceipt](https://github.com/streamingfast/firehose-ethereum/blob/develop/proto/sf/ethereum/type/v2/type.proto#L296) struct representing the receipt of the transaction.
- `status()` (method): [TransactionTraceStatus](https://github.com/streamingfast/firehose-ethereum/blob/develop/proto/sf/ethereum/type/v2/type.proto#L289) enum struct representing the status of the transaction, with the following possible values: `TransactionTraceStatus::Succeeded`, `TransactionTraceStatus::Failed`, `TransactionTraceStatus::Reverted`, `TransactionTraceStatus::Unknown`.


<figure><img src="../../.gitbook/assets/cheatsheet/cheatsheet-transaction-structure.png" width="100%" /><figcaption><p>EVM-compatible Protobuf Structure - TransactionTrace</p></figcaption></figure>

# Iterating over ALL Transactions

```rust
use substreams::Hex;
use substreams_ethereum::pb::eth::v2::Block;

struct TransactionMeta {
    hash: String,
    from: String,
    to: String
}

fn all_transactions(blk: Block) -> Vec<TransactionMeta> {
    return blk.transaction_traces
            .iter()
            .map(|tx| TransactionMeta {
                hash: Hex::encode(tx.hash),
                from: Hex::encode(tx.from),
                to: Hex::encode(tx.to)
            })
            .collect();
}
```

# Iterating over SUCCESSFUL Transactions

```rust
use substreams::Hex;
use substreams_ethereum::pb::eth::v2::Block;

struct TransactionMeta {
    hash: String,
    from: String,
    to: String
}

fn successful_transactions(blk: Block) -> Vec<TransactionMeta> {
    return blk.transactions()
        .map(|tx| TransactionMeta {
            hash: Hex::encode(tx.hash),
            from: Hex::encode(tx.from),
            to: Hex::encode(tx.to)
        })
        .collect();
}
```

or

```rust
use substreams::Hex;
use substreams_ethereum::pb::eth::v2::{Block, TransactionTraceStatus};

struct TransactionMeta {
    hash: String,
    from: String,
    to: String
}

fn successful_transactions(blk: Block) -> Vec<TransactionMeta> {
    return blk.transaction_traces.iter()
        .filter(|tx| tx.status() == TransactionTraceStatus::Succeeded)
        .map(|tx| TransactionMeta {
            hash: Hex::encode(tx.hash),
            from: Hex::encode(tx.from),
            to: Hex::encode(tx.to)
        })
        .collect();
}
```

# Iterating over FAILED Transactions

```rust
use substreams::Hex;
use substreams_ethereum::pb::eth::v2::{Block, TransactionTraceStatus};

struct TransactionMeta {
    hash: String,
    from: String,
    to: String
}

fn failed_transactions(blk: Block) -> Vec<TransactionMeta> {
    return blk.transaction_traces.iter()
        .filter(|tx| tx.status() == TransactionTraceStatus::Failed)
        .map(|tx| TransactionMeta {
            hash: Hex::encode(tx.hash),
            from: Hex::encode(tx.from),
            to: Hex::encode(tx.to)
        })
        .collect();
}
```


# Iterating over REVERTED Transactions

```rust
use substreams::Hex;
use substreams_ethereum::pb::eth::v2::{Block, TransactionTraceStatus};

struct TransactionMeta {
    hash: String,
    from: String,
    to: String
}

fn failed_transactions(blk: Block) -> Vec<TransactionMeta> {
    return blk.transaction_traces.iter()
        .filter(|tx| tx.status() == TransactionTraceStatus::Reverted)
        .map(|tx| TransactionMeta {
            hash: Hex::encode(tx.hash),
            from: Hex::encode(tx.from),
            to: Hex::encode(tx.to)
        })
        .collect();
}
```
