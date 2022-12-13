---
description: StreamingFast Substreams protobuf schemas
---

# Protobuf Schemas

### Protobuf Overview

Substreams uses Protocol Buffers (protobufs) as the API for data models specific to each blockchain. Each manifest defines references to the protobufs for the Substreams implementation.&#x20;

{% hint style="success" %}
**Tip**: Protobufs define the input and output for modules.
{% endhint %}

### Protobuf Basics

Protobufs are Google's language-neutral extensible mechanism for serializing structured data. Protobufs are similar to XML but smaller, faster, and simpler.&#x20;

Additional information can be found for Protocol Buffers by visiting the links provided below.&#x20;

**Google Protocol Buffer Documentation**

[https://developers.google.com/protocol-buffers](https://developers.google.com/protocol-buffers)

**Google Protocol Buffer Tutorial**

[https://developers.google.com/protocol-buffers/docs/tutorials](https://developers.google.com/protocol-buffers/docs/tutorials)

### Protobuf Definition

Define a protobuf model as [`proto:eth.erc721.v1.Transfers`](https://github.com/streamingfast/substreams-template/blob/develop/proto/erc721.proto) representing a list of ERC721 transfers.

{% hint style="info" %}
**Note**: The Transfers protobuf in the Substreams Template example is located in the proto directory.
{% endhint %}

{% code title="eth/erc721/v1/erc721.proto" lineNumbers="true" %}

```protobuf
syntax = "proto3";

package eth.erc721.v1;

message Transfers {
  repeated Transfer transfers = 1;
}

message Transfer {
  bytes from = 1;
  bytes to = 2;
  uint64 token_id = 3;
  bytes trx_hash = 4;
  uint64 ordinal = 5;
}
```

{% endcode %}

View this file in the repo by visiting the following link.

[https://github.com/streamingfast/substreams-template/blob/develop/proto/erc721.proto](https://github.com/streamingfast/substreams-template/blob/develop/proto/erc721.proto)

#### Identifying Data types

The ERC721 smart contract associated with the Substreams Template example contains a Transfer event. The event is targeted by creating an associated protobuf.&#x20;

The protobuf file serves as the interface between the module handlers and the data being provided by Substreams.&#x20;

{% hint style="success" %}
**Tip**: Protobufs are chain agnostic and can be defined and used for various blockchains. The ERC721 smart contracts used in the Substreams Template example are generic contracts used across many different Ethereum applications. The size and scope of the Substreams implementation will dictate the number of and complexity of protobufs.
{% endhint %}

{% hint style="info" %}
**Note**: The Substreams Template example targets Transfer events associated with the Bored Ape Yacht Club smart contract, located on the Ethereum blockchain.&#x20;
{% endhint %}

Multitudes of more specific data types exist in the Ethereum smart contract ecosystem, some extending the ERC20 and ERC721 base implementations. Developers can create more refined and complex protobufs based on the many custom data types that exist in the blockchain they are targeting.

{% hint style="success" %}
**Tip**_:_ Using fully qualified paths for protobuf files reduces the risk of naming conflicts when other community members build their [Substreams packages](../reference-and-specs/packages.md#dependencies).
{% endhint %}

### Generating Protobufs

The Substreams CLI is used to generate the associated Rust code for the protobuf.

{% hint style="success" %}
**Tip**: Notice the `protogen` command and Substreams manifest passed into the CLI.
{% endhint %}

{% code overflow="wrap" %}

```bash
substreams protogen ./substreams.yaml --exclude-paths="sf/ethereum,sf/substreams,google"
```

{% endcode %}

The Rust code is generated and saved into [`src/pb/eth.erc721.v1.rs`](https://github.com/streamingfast/substreams-template/blob/develop/src/pb/eth.erc721.v1.rs)``

The [`mod.rs`](https://github.com/streamingfast/substreams-template/blob/develop/src/pb/mod.rs) file located in the `src/pb` directory of the Substreams Template example is responsible for exporting the freshly generated Rust code.

{% code title="src/pb/mod.rs" %}

```rust
#[path = "eth.erc721.v1.rs"]
#[allow(dead_code)]
pub mod erc721;
```

{% endcode %}

View this file in the repo by visiting the following link.

[https://github.com/streamingfast/substreams-template/blob/develop/src/pb/mod.rs](https://github.com/streamingfast/substreams-template/blob/develop/src/pb/mod.rs)

### Protobuf & Rust Optional Fields

Each field in a Protocol Buffer message are optional by default. Each field in a Protocol Buffer message needs a default value, this indicates that the field has not been populated with any data.

For each field that are reference to other Protocol Buffer message types, Prost generates Rust code that uses the `Option` enum. The `Option` enum is used to represent the absence of a value in Rust. It allows developers to distinguish between a field that has a value and a field that has not been set. The standard approach to represent nullable data when using Rust is through wrapping optional values in `Option<T>`.

Additional information is available for `prost` in its [official GitHub repository](https://github.com/tokio-rs/prost).

Learn more about Rust Option[Rust Option](https://doc.rust-lang.org/rust-by-example/std/option.html) in the offical documentation.
