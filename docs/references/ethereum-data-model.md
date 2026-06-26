# The EVM Data Model

The [sf.ethereum.type.v2.Block](https://github.com/streamingfast/firehose-ethereum/blob/develop/proto/sf/ethereum/type/v2/type.proto#L51) Protobuf is shared across EVM-compatible blockchains (Ethereum, Polygon, Arbitrum...). While the Protobuf definition (`.proto` file) is largely self-documented, it is important to be aware of the **version** of the Protobuf you are working with.

## The Protobuf Version

Most changes in EVM chains are backward-compatible, meaning there is no need to introduce an entirely new Protobuf namespace (e.g., _sf.ethereum.type.v3.Block_). Instead, `sf.ethereum.type.v2.Block` is used consistently to avoid breaking changes.

To manage internal updates, Firehose uses a versioning system defined by the `ver` [field](https://github.com/streamingfast/firehose-ethereum/blob/develop/proto/sf/ethereum/type/v2/type.proto#L132) in the `Block` Protobuf.

```rust
// Ver represents that data model version of the block, it is used internally by Firehose on Ethereum
// as a validation that we are reading the correct version.
int32 ver = 1;
```

This version refers to the `Block` emitted by the Firehose trace.

### Version 3

- This version is current in place for **all EVM chains, with the exception of Optimism** from block 0 to the last pre-Prague (blockchains currently in Prague version are listed in a section later in this document) hard fork block.
- This version contains several known issues, which are described in the Protobuf definition itself (next to the corresponding field affected). You can also check out [this GitHub issue](https://github.com/streamingfast/firehose-ethereum/issues/71).

### Version 4

- This version is currently in place for the following chains:
    - Optimism
    - Ethereum Hoodi
- This version fixes the known issue of version 3.

## Prague-enabled blockchains

The following chains have already upgraded to Ethereum's Prague version:
- Ethereum Sepolia
- Ethereum Holesky
- BSC Mainnet
- BSC Testnet