---
description: StreamingFast Substreams protobuf schemas
---

# Protobuf schemas

### Protobuf overview

Substreams uses Protocol Buffers (protobufs) as the API for data models specific to the different blockchains. Manifests define references to the protobufs for the Substreams module.

{% hint style="success" %}
**Tip**: Protobufs define the input and output for modules.
{% endhint %}

### Protobuf Basics

> Protobufs are Google's language-neutral extensible mechanism for serializing structured data.

**Google Protocol Buffer Documentation**

[https://developers.google.com/protocol-buffers](https://developers.google.com/protocol-buffers)

**Google Protocol Buffer Tutorial**

[https://developers.google.com/protocol-buffers/docs/tutorials](https://developers.google.com/protocol-buffers/docs/tutorials)

### Protobuf definition

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

View this file in the repository:

[https://github.com/streamingfast/substreams-template/blob/develop/proto/erc721.proto](https://github.com/streamingfast/substreams-template/blob/develop/proto/erc721.proto)

#### Identifying data types

The ERC721 smart contract associated with the Substreams Template example contains a Transfer event. The event is targeted by creating an associated protobuf.

The protobuf file serves as the interface between the module handlers and the data being provided by Substreams.

{% hint style="success" %}
**Tip**: Protobufs are platform-independent and are defined and used for various blockchains. The ERC721 smart contracts used in the Substreams Template example are generic contracts used across many different Ethereum applications. The size and scope of the Substreams implementation will dictate the number of and complexity of protobufs.
{% endhint %}

{% hint style="info" %}
**Note**: The Substreams Template example targets Transfer events associated with the Bored Ape Yacht Club smart contract, located on the Ethereum blockchain.
{% endhint %}

Several specific data types exist in the Ethereum smart contract ecosystem, some extending the ERC20 and ERC721 base implementations. Developers will create more refined and complex protobufs based on the many custom data types that exist in the blockchain they are targeting.

{% hint style="success" %}
**Tip**_:_ Using fully qualified paths for protobuf files reduces the risk of naming conflicts when other community members build their [Substreams packages](../reference-and-specs/packages.md#dependencies).
{% endhint %}

### Generating protobufs

The Substreams CLI is used to generate the associated Rust code for the protobuf.

{% hint style="success" %}
**Tip**: Notice the `protogen` command and Substreams manifest passed into the Substreams CLI.
{% endhint %}

{% code overflow="wrap" %}
```bash
substreams protogen ./substreams.yaml --exclude-paths="sf/ethereum,sf/substreams,google"
```
{% endcode %}

The Rust code is generated and saved into [`src/pb/eth.erc721.v1.rs`](https://github.com/streamingfast/substreams-template/blob/develop/src/pb/eth.erc721.v1.rs)

The [`mod.rs`](https://github.com/streamingfast/substreams-template/blob/develop/src/pb/mod.rs) file located in the `src/pb` directory of the Substreams Template example is responsible for exporting the freshly generated Rust code.

{% code title="src/pb/mod.rs" %}
```rust
#[path = "eth.erc721.v1.rs"]
#[allow(dead_code)]
pub mod erc721;
```
{% endcode %}

View this file in the repository:

[https://github.com/streamingfast/substreams-template/blob/develop/src/pb/mod.rs](https://github.com/streamingfast/substreams-template/blob/develop/src/pb/mod.rs)

### Protobuf and Rust optional fields

Protocol buffers define fields' type using standard primitive data types, such as integers, booleans, and floats or a complex data type such as `message`, `enum`, `oneof` or `map`. View the [full list](https://developers.google.com/protocol-buffers/docs/proto#scalar) of types in the Google Protocol Buffers documentation.

Any primitive data types in a message will generate the corresponding Rust type,`String` for `string`, `u64` for `uint64,` and will assign the default value of the corresponding Rust type if the field is not present in a message, an empty string for `String`, 0 for integer types, `false` for `bool`. For fields that reference other complex `messages` Rust generates the corresponding `message` type wrapped with an `Option` enum type, and will use the `None` variant if the field is not present in the message.

The `Option` enum is used to represent the presence (`Some(x)`) or absence (`None`) of a value in Rust. It allows developers to distinguish between a field containing a value and a field that has not been set. The standard approach to represent nullable data when using Rust is by wrapping optional values in `Option<T>`.

The Rust `match` keyword is used to compare the value of an `Option` with a `Some` or `None` variant. Handle a type wrapped `Option` using:

```rust
match person.Location {
    Some(location) => { /* Value is present, do something */ }
    None => { /* Value is absent, do something */ }
}
```

If you are only interested in dealing with the presence of a value, use the `if let` statement to handle the `Some(x)` arm of the `match` code.

```rust
if let Some(location) = person.location {
    // Value is present, do something
}
```

If a value is present use the `.unwrap()` call on the `Option` to obtain the wrapped data. This is true if you control the creation of the messages yourself or if the field is documented as always being present

{% hint style="info" %}
**Note**: Be 100% sure that the field is always present, otherwise Substreams will panic and never complete, being stuck on this block forever.
{% endhint %}

Additional information is available for `prost`, the tool generating the Rust code from Protobuf definitions, in its official GitHub repository.

[https://github.com/tokio-rs/prost](https://github.com/tokio-rs/prost)

_Learn more about_[ _Option_](https://doc.rust-lang.org/rust-by-example/std/option.html) _in the official Rust documentation._
