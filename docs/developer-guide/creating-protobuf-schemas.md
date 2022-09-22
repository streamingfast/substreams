---
description: StreamingFast Substreams protobuf schemas
---

# Protobuf Schemas

### Protobuf Overview

After creating the Substreams manifest a custom protobuf needs to be defined in the manifest file. The protobuf is used for defining input and output.

Protocol Buffers are Google's language-neutral extensible mechanism for serializing structured data. They are like XML, but smaller, faster, and simpler. Find additional information regarding Protocol Buffers on the Google website.

#### Google Protobuf Documentation

[https://developers.google.com/protocol-buffers](https://developers.google.com/protocol-buffers)

#### Google Protobuf Tutorial

[https://developers.google.com/protocol-buffers/docs/tutorials](https://developers.google.com/protocol-buffers/docs/tutorials)

### Protofuf Definition

Define a protobuf model as `proto:eth.erc721.v1.Transfers` representing a list of ERC721 transfers.

Create a proto directory in the substreams directory and then create the protobuf definition file.

{% code title="eth/erc721/v1/erc721.proto" %}
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

{% hint style="success" %}
Use a fully qualified path for protobuf files to reduce the risk of conflicts when other community members build their own [_Substreams Packages_](../reference-and-specs/packages.md#dependencies)_._
{% endhint %}

Next, generate the associated Rust code for the protobuf.

```bash
substreams protogen ./substreams.yaml --exclude-paths="sf/ethereum,sf/substreams,google"
```

The generated Rust code will be created as `src/pb/eth.erc721.v1.rs`

Next, add a `mod.rs` file in the `src/pb` directory to export the newly generated Rust code.

{% code title="src/pb/mod.rs" %}
```rust
#[path = "eth.erc721.v1.rs"]
#[allow(dead_code)]
pub mod erc721;
```
{% endcode %}
