---
description: StreamingFast Substreams manifest reference
---

# Manifests

The Substreams Manifest, `substreams.yaml`, defines the modules composing the Substreams. The manifest is primarily used to define the dependencies between the inputs and outputs of modules.

Below is a reference guide of _all_ fields used in Substreams manifests.

### Specification Version

Excerpt pulled from the example Substreams manifest.

```yaml
specVersion: v0.1.0
```

Simply use `v0.1.0` for the `specVersion` field.

### Package

Excerpt pulled from the example Substreams manifest.

```yaml
package:
  name: module_name_for_implementation
  version: v0.5.0
  url: https://github.com/streamingfast/substreams-playground
  doc: |
    Documentation heading for the package.

    More detailed docs for the package.
```

#### Package Name

The `package.name` field is used to identify the package.&#x20;

The `package.name` field also infers the filename when the `pack` command is run using `substreams.yaml` as a flag for the Substreams package.

* The `name` must match the following regular expression. \
  `^([a-zA-Z][a-zA-Z0-9_]{0,63})$`
* The regular expression translates to the following rules.
  * 64 characters maximum
  * Separate words with `_`
  * Starts with `a-z` or `A-Z` and can contain numbers thereafter

#### Package Version

The `package.version` field identifies the package for the Substreams implementation.

{% hint style="info" %}
**Note**: `package.version` _must_ respect [Semantic Versioning, version 2.0](https://semver.org/)
{% endhint %}

#### Package URL

The `package.url` field identifies and helps users discover the source of the Substreams package.

#### Package Doc

The `package.doc` field is the documentation string of the package. The first line is a short description; longer documentation should follow a blank line.

### Imports

The `imports` section references WASM code, compiled protobuf definitions, and module definitions.&#x20;

{% hint style="success" %}
**Tip**: Imported modules can be referred to using a key later in the `modules` section.
{% endhint %}

Excerpt pulled from the example Substreams manifest.

```yaml
imports:
  ethereum: substreams-ethereum-v1.0.0.spkg
  tokens: ../eth-token/substreams.yaml
  prices: ../eth-token/substreams.yaml
```

The _value_ should be a pointer to a Substreams manifest or a Substreams [package](packages.md).

The filename can be absolute or relative or a remote path starting with `http://` or `https://`.

Imports will differ across each blockchain. For example, Substreams implementations that target Ethereum will reference an appropriate spkg file created for that blockchain. Solana, and other blockchains, will reference a different spkg or resources specific to the target chain.

### Protobuf

The `protobuf` section points to the Protocol Buffer definitions used by the modules in the Substreams implementation.

Excerpt pulled from the example Substreams manifest.

```yaml
protobuf:
  files:
    - google/protobuf/timestamp.proto
    - pcs/v1/pcs.proto
    - pcs/v1/database.proto
  importPaths:
    - ./proto
    - ../../external-proto
```

The Substreams packager will load files in any of the listed `importPaths`.

{% hint style="info" %}
**Note**: The `imports` section of the manfiest will also affect which `.proto` files end up in the package.
{% endhint %}

They are packaged with the modules to help clients decode the incoming streams, but are not sent to Substreams server in network requests.

Refer to [standard protobuf documentation](https://developers.google.com/protocol-buffers/docs/proto3) for more information about Protocol Buffers.

### Binaries

The `binaries` field specifies the binary code to use when executing modules.&#x20;

The field `modules[].binary` has a default value of `default`.&#x20;

{% hint style="info" %}
**Note**_: defining the `default` binary is a crucial step of the process when working with Substreams manifests._
{% endhint %}

Excerpt pulled from the example Substreams manifest.

```yaml
binaries:
  default:
    type: wasm/rust-v1
    file: ./target/wasm32-unknown-unknown/release/my_package.wasm
  other:
    type: wasm/rust-v1
    file: ./snapshot_of_my_package.wasm
```

You can override which binary to use in the [`modules` section](manifests.md#undefined) (see below), and define other binaries by their name (like `other` in the example above).

#### `binaries[name].type`

The type of code and implied VM for execution.

{% hint style="info" %}
_Note, at the time of writing, there is only one VM available and it's value is: `wasm/rust-v1`._
{% endhint %}

#### `binaries[name].file`

The path pointing to a local compiled [WASM module](https://webassembly.github.io/spec/core/syntax/modules.html). The path will be absolute or relative to the current `.yaml` file's directory.

This file will be picked up and packaged into an `.spkg` when invoking the Substreams `pack` and `run` commands.

### Modules

Excerpt pulled from the example Substreams manifest.

```yaml
  - name: events_extractor
    kind: map
    initialBlock: 5000000
    binary: default  # Implicit
    inputs:
      - source: sf.ethereum.type.v2.Block
      - store: myimport:prices
    output:
      type: proto:my.types.v1.Events

  - name: totals
    kind: store
    updatePolicy: add
    valueType: int64
    inputs:
      - source: sf.ethereum.type.v2.Block
      - map: events_extractor
```

#### `modules[].name`

The identifier for the module, starting with a letter, followed by a maximum of 64 characters of `[a-zA-Z0-9_]`. The same rules apply for the `package.name` field.

It is the reference identifier used on the command line and in [`inputs`](manifests.md#modules-.inputs). Each package should have a unique name.

{% hint style="info" %}
_Note: `modules[].name` also corresponds to the **name of the Rust function** that will be invoked on the compiled WASM code upon execution. It is the same function that will be defined. `#[substreams::handlers::map]`(or`store`) in your Rust code._
{% endhint %}

{% hint style="success" %}
_Tip: When importing another package, all module names will be prefixed with the package's name and a colon. This prefixing ensures that there will be no name clashes across multiple imported packages and nearly any names can be safely used._
{% endhint %}

#### `modules[].initialBlock`

The initial block for the module is where Substreams is will begin processing data for a particular module. The runtime will simply never process blocks prior to the one for any given module.

If all the inputs have the same `initialBlock` the field can be omitted and its value will be inferred by its dependent [`inputs`](manifests.md#modules-.inputs).

`initialBlock` becomes mandatory when inputs have _different_ values.

#### `modules[].kind`

There are two module types associated with `modules[].kind` as indicated below.

* `map`
* `store`

#### `modules[].updatePolicy`

Valid only for `kind: store`.

Specifies the merge strategy for two contiguous partial stores produced by parallelized operations.&#x20;

Possible values for `modules[].updatePolicy` are as follows.

* `set` (last key wins merge strategy)
* `set_if_not_exists` (first key wins merge strategy)
* `append` (concatenates two keys' values)
* `add` (sum the two keys' values)
* `min` (min between two keys' values)
* `max` (max between two keys' values)

#### `modules[].valueType`

Valid only for `kind: store`.

Specifies the data type of all keys in the `store`, and determines the WASM imports available to the module to write to the store.&#x20;

Possible values for `modules[].valueTypes` are as follows.

* `bigfloat`
* `bigint`
* `int64`
* `bytes`
* `string`
* `proto:path.to.custom.protobuf.Model`

#### `modules[].binary`

An identifier defined in the [`binaries`](manifests.md#binaries) section.

The `modules[].binary` module will execute using the code provided. This allows multiple WASM definitions for different modules enabling caching while iterating on the WASM code.

#### `modules[].inputs`

Excerpt pulled from the example Substreams manifest.

```yaml
inputs:
    - source: sf.ethereum.type.v2.Block
    - store: my_store
      mode: deltas
    - store: my_store # defaults to mode: get
    - map: my_map
```

`inputs` is a list of _input_ structures. For each object, one of three keys is required. The inputs key types are:

* `source,`
* `store` (also used to define `mode` keys),
* and `map`.

#### `modules[].output`

Valid only for `kind: map`.

Excerpt pulled from the example Substreams manifest.

```yaml
output:
    type: proto:eth.erc721.v1.Transfers
```

The value for `type` will always be prefixed with `proto:` followed by a definition specified in the protobuf definitions, and referenced in the [`protobuf`](manifests.md#protobuf) section.
