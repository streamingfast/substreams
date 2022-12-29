---
description: StreamingFast Substreams protobuf schemas
---

# Protobuf schemas

### Protobuf overview

Substreams uses Google Protocol Buffers extensively. Protocol Buffers (protobufs) are used as the API for data models specific to the different blockchains. Manifests contain references to the protobufs for your Substreams module.

{% hint style="success" %}
**Tip**: Protobufs define the input and output for modules.
{% endhint %}

Learn more about the details of Google Protocol Buffers in the official documentation provided by Google.

**Google Protocol Buffer Documentation**

[https://developers.google.com/protocol-buffers](https://developers.google.com/protocol-buffers)

**Google Protocol Buffer Tutorial**

[https://developers.google.com/protocol-buffers/docs/tutorials](https://developers.google.com/protocol-buffers/docs/tutorials)

### Protobuf definition for Substreams

Define a protobuf model as [`proto:eth.erc721.v1.Transfers`](https://github.com/streamingfast/substreams-template/blob/develop/proto/erc721.proto) representing a list of ERC721 transfers.

{% hint style="info" %}
**Note**: The `Transfers` protobuf in the Substreams Template example is located in the proto directory.
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

View the `erc721.proto` file in the repository:

[https://github.com/streamingfast/substreams-template/blob/develop/proto/erc721.proto](https://github.com/streamingfast/substreams-template/blob/develop/proto/erc721.proto)

#### Identifying data types

The ERC721 smart contract used in the Substreams Template example contains a `Transfer` event. You can use the event data through a custom protobuf.

The protobuf file serves as the interface between the module handlers and the data being provided by Substreams.

{% hint style="success" %}
**Tip**: Protobufs are platform-independent and are defined and used for various blockchains.&#x20;

* The ERC721 smart contracts used in the Substreams Template example are generic contracts used across many different Ethereum applications.&#x20;
* The size and scope of the Substreams implementation dictates the number of and complexity of protobufs.
{% endhint %}

{% hint style="info" %}
**Note**: The Substreams Template example extracts `Transfer` events from the Bored Ape Yacht Club smart contract which is located on the Ethereum blockchain.
{% endhint %}

Several specific data types exist in the Ethereum smart contract ecosystem, some extending the ERC20 and ERC721 base implementations. Complex protobufs are created and refined based on the various data types used across the different blockchains.

{% hint style="success" %}
**Tip**_:_ The use of fully qualified protobuf file paths reduces the risk of naming conflicts when other community members build their [Substreams packages](../reference-and-specs/packages.md#dependencies).
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

View the `mod.rs` file in the repository:

[https://github.com/streamingfast/substreams-template/blob/develop/src/pb/mod.rs](https://github.com/streamingfast/substreams-template/blob/develop/src/pb/mod.rs)

### Protobuf and Rust optional fields

Protocol buffers define fields' type by using standard primitive data types, such as integers, booleans, and floats or a complex data type such as `message`, `enum`, `oneof` or `map`. View the [full list](https://developers.google.com/protocol-buffers/docs/proto#scalar) of types in the Google Protocol Buffers documentation.

Any primitive data types in a message generate the corresponding Rust type,`String` for `string`, `u64` for `uint64,` and assign the default value of the corresponding Rust type if the field is not present in a message, an empty string for `String`, 0 for integer types, `false` for `bool`. &#x20;

Rust generates the corresponding `message` type wrapped by an `Option` enum type for fields referencing other complex `messages`. The `None` variant is used if the field is not present in the message.

The `Option` enum is used to represent the presence (`Some(x)`) or absence (`None`) of a value in Rust. `Option` allows developers to distinguish between a field containing a value versus a field not assigned a value.&#x20;

{% hint style="info" %}
**Note**: The standard approach to represent nullable data in Rust is to wrap optional values in `Option<T>`.
{% endhint %}

The Rust `match` keyword is used to compare the value of an `Option` to a `Some` or `None` variant. Handle a type wrapped `Option` in Rust by using:

```rust
match person.Location {
    Some(location) => { /* Value is present, do something */ }
    None => { /* Value is absent, do something */ }
}
```

If you are only interested in finding the presence of a value, use the `if let` statement to handle the `Some(x)` arm of the `match` code.

```rust
if let Some(location) = person.location {
    // Value is present, do something
}
```

If a value is present, use the `.unwrap()` call on the `Option` to obtain the wrapped data. You'll need to account for these types of scenarios if you control the creation of the messages yourself or if the field is documented as always being present.

{% hint style="info" %}
**Note**: You need to be absolutely sure the field is always defined, otherwise Substreams panics and never completes, being stuck on a block indefinitely.
{% endhint %}

_**PROST!**_ is a tool for generating Rust code from Protobuf definitions. Additional information for `prost` is available in the project's official GitHub repository.

[https://github.com/tokio-rs/prost](https://github.com/tokio-rs/prost)

Learn more about[ Option](https://doc.rust-lang.org/rust-by-example/std/option.html) in the official Rust documentation.

[https://doc.rust-lang.org/rust-by-example/std/option.html](https://doc.rust-lang.org/rust-by-example/std/option.html)
