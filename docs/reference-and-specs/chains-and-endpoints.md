---
description: StreamingFast Substreams chains and endpoints
---

# Chains and endpoints

## Chains and endpoints overview

The different blockchains have separate endpoints that Substreams uses. You will use the endpoint that matches the blockchain you've selected for your development initiative.

### Supported blockchains and protobuf models

Protobuf definitions and public endpoints are provided for the supported protocols and chains.

{% hint style="success" %}
**Tip**: All of the endpoints listed in the documentation require [authentication](authentication.md) before use.
{% endhint %}

{% hint style="warning" %}
**Important**_:_ Endpoints serve protobuf models specific to the underlying blockchain protocol and must match the `source:` field for the module.

**Streaming a `sf.near.type.v1.Block` from an Ethereum endpoint does not work!**
{% endhint %}

| Protocol | Proto model                                                                                                                                     | Latest package                                                                                                        |
| -------- | ----------------------------------------------------------------------------------------------------------------------------------------------- | --------------------------------------------------------------------------------------------------------------------- |
| Ethereum | [`sf.ethereum.type.v2.Block`](https://github.com/streamingfast/firehose-ethereum/blob/develop/proto/sf/ethereum/type/v2/type.proto)             | [ethereum-v0.10.4.spkg](https://github.com/streamingfast/sf-ethereum/releases/download/v0.10.2/ethereum-v0.10.4.spkg) |
| NEAR     | [`sf.near.type.v1.Block`](https://github.com/streamingfast/firehose-near/blob/develop/proto/sf/near/type/v1/type.proto)                         |                                                                                                                       |
| Solana   | [`sf.solana.type.v1.Block`](https://github.com/streamingfast/firehose-solana/blob/develop/proto/sf/solana/type/v1/type.proto)                   | [solana-v0.1.0.spkg](https://github.com/streamingfast/sf-solana/releases/download/v0.1.0/solana-v0.1.0.spkg)          |
| Cosmos   | [`sf.cosmos.type.v1.Block`](https://github.com/figment-networks/proto-cosmos/blob/main/sf/cosmos/type/v1/type.proto)                            |                                                                                                                       |
| Arweave  | [`sf.arweave.type.v1.Block`](https://github.com/streamingfast/firehose-arweave/blob/develop/proto/sf/arweave/type/v1/type.proto)``              |                                                                                                                       |
| Aptos    | [`aptos.extractor.v1.Block`](https://github.com/aptos-labs/aptos-core/blob/main/crates/aptos-protos/proto/aptos/extractor/v1/extractor.proto)`` |                                                                                                                       |

## Endpoints

* **Ethereum Mainnet**: `mainnet.eth.streamingfast.io:443`
* **Ethereum Görli**: `goerli.eth.streamingfast.io:443`
* **Polygon** **Mainnet**: `polygon.streamingfast.io:443`
* **BNB**: `bnb.streamingfast.io:443`
* **NEAR Mainnet**: `mainnet.near.streamingfast.io:443`
* **NEAR Testnet**: `testnet.near.streamingfast.io:443`
* **Solana mainnet-beta**: `mainnet.sol.streamingfast.io:443`
* **Arweave Mainnet**: `mainnet.arweave.streamingfast.io:443`
* **Aptos Testnet**: `testnet.aptos.streamingfast.io:443`

## Others

You can support other blockchains for Substreams through Firehose instrumentation. Learn more in the [official Firehose documentation](https://firehose.streamingfast.io/).
