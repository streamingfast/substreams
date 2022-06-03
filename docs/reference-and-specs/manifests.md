# Manifests

The Substreams Manifest, `substreams.yaml`, defines the modules composing the Substreams. The manifest is used, among other things, to define the dependencies between your module's inputs and outputs.

Below is a reference guide of all fields in a manifest YAML file.

## `specVersion`

Example:

```yaml
specVersion: v0.1.0
```

Just make it `v0.1.0` - no questions asked.

## `package`

Example:

```yaml
package:
  name: my_module_name
  version: v0.5.0
  url: https://github.com/streamingfast/substreams-playground
  doc: |
    This is the heading of the documentation for this package.

    This is more detailed docs for this package.
```

### `package.name`

This field is used to identify your package, and is used to infer the filename when you  `substreams pack substreams.yaml` your package.

* `name` must match this regular expression: `^([a-zA-Z][a-zA-Z0-9_]{0,63})$`, meaning:
* 64 characters maximum
* Separate words with `_`
* Starts with `a-z` or `A-Z` and can contain numbers thereafter

### `package.version`

This field identifies the package revision. It must respect [Semantic Versioning version 2.0](https://semver.org/)

### `package.url`

This field helps your users discover the source of the package.

### `package.doc`

This field holds the documentation string of the package.

The first line is a short description. Longer documentation follows a blank line.

## `imports`

Example:

```yaml
imports:
  ethereum: substreams-ethereum-v1.0.0.spkg
  tokens: ../eth-token/substreams.yaml
  prices: ../eth-token/substreams.yaml
```

The `imports` section imports modules with their WASM code, all of their (compiled) protobuf definitions and modules definition. The imported modules can be referred to by the _key_ later in the `modules` section.

The _value_ should be a pointer to either a YAML manifest for Substreams Modules (ending in `.yaml`), or a [Package](packages.md) (ending in `.spkg`).

The filename can be an absolute, relative (to the location of the `.yaml` file), or remote path as long as it starts with `http://` or `https://`.

## `Protobuf`

Example:

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

The `Protobuf` section points to the protobuf definitions used by these modules.

The Substreams packager will load files in any of the listed `importPaths`.\
Note that the `imports` section will also affect which `.proto` files end up in your package.

They are packaged with the modules to help clients decode the incoming streams, but are not sent to Substreams server in network requests.

Refer to [standard protobuf documentation](https://developers.google.com/protocol-buffers/docs/proto3) for more information about Protocol Buffers.

## `binaries`

Example:

```yaml
binaries:
  default:
    type: wasm/rust-v1
    file: ./target/wasm32-unknown-unknown/release/my_package.wasm
  other:
    type: wasm/rust-v1
    file: ./snapshot_of_my_package.wasm
```

This specifies the binary code to use when executing modules. The field [`modules[].binary`](manifests.md#modules-.binary) has a default value of `default`. Therefore, make sure to define the `default` binary here.

You can override which binary to use in the [`modules` section](manifests.md#undefined) (see below), and define other binaries by their name (like `other` in the example above).

### `binaries[name].type`

The type of code, and the implied VM for execution.

At the moment, there is only one VM available, so the value here should be `wasm/rust-v1`

### `binaries[name].file`

A path pointing to a local compiled [WASM Module](https://webassembly.github.io/spec/core/syntax/modules.html). It can be an absolute path, or relative to the current `.yaml` file's directory.

This file will be picked up and packaged into an `.spkg` when invoking `substreams pack`, as well as any `substreams run`.

## `modules`

Examples:

```yaml
  - name: events_extractor
    kind: map
    initialBlock: 5000000
    binary: default  # Implicit
    inputs:
      - source: sf.ethereum.type.v1.Block
      - store: myimport:prices
    output:
      type: proto:my.types.v1.Events

  - name: totals
    kind: store
    updatePolicy: add
    valueType: int64
    inputs:
      - source: sf.ethereum.type.v1.Block
      - map: events_extractor
```

### `modules[].name`

The identifier for the module, starting with a letter, followed by a maximum of 64 characters of `[a-zA-Z0-9_]`. These are the same rules as for `package.name`.

It is the reference identifier used on the command line and in [`inputs`](manifests.md#modules-.inputs). Each package should have a unique name.

{% hint style="info" %}
This `name` also corresponds to the **Rust function name** that will be invoked on the compiled WASM code upon execution. This is the same function you will define `#[substreams::handlers::map]`(or`store`) in your Rust code.
{% endhint %}

{% hint style="success" %}
When importing another package, all of its modules' names will be prefixed with the package's name and a colon. This way, there are no name clashes across imported packages, and you can safely reuse the same names in your manifest.
{% endhint %}

### `modules[].initialBlock`

The initial block for the module is where your Substreams is going to start processing data for that particular module. The runtime will simply never process blocks prior to this one for the given module.

The `initialBlock` field can be elided and its value will be inferred by its dependent [`inputs`](manifests.md#modules-.inputs), as long as all the inputs have the same `initialBlock`. If some _inputs_ have different `initialBlock`, then it becomes mandatory.

### `modules[].kind`

The type of `module`. There are two types of modules:

* `map`
* `store`

Learn more about modules [here](../concepts/modules.md)

### `modules[].updatePolicy`

Valid only for `kind: store`.

Specifies the merge strategy for two contiguous partial stores produced by parallelized operations. See [Modules](../concepts/modules.md#writing) for details.

Possible values:

* `set` (last key wins merge strategy)
* `set_if_not_exists` (first key wins merge strategy)
* `add` (sum the two keys)
* `min` (min between two keys)
* `max` (max between two keys)

### `modules[].valueType`

Valid only for `kind: store`.

Specifies the data type of all keys in the `store`, and determines the WASM imports available to the module to write to the store. See [API Reference](rust-api.md) for details.

Possible values:

* `bigfloat`
* `bigint`
* `int64`
* `bytes`
* `string`
* `proto:some.path.to.protobuf.Model`

### `modules[].binary`

An identifier defined in the [`binaries`](manifests.md#binaries) section.

This module will execute using the code specified, allowing you to have multiple WASM for different modules, and allowing you to leverage caching while iterating on your WASM code.

### `modules[].inputs`

Example:

```yaml
inputs:
    - source: sf.ethereum.type.v1.Block
    - store: my_store
      mode: deltas
    - store: my_store # defaults to mode: get
    - map: my_map
```

`inputs` is a list of _input_ structures. For each object, one of three keys is required:

* `source`
* `store` (can also define a `mode` key)
* `map`

See [Module Inputs](../concept-and-fundamentals/modules/inputs.md) for details.

### `modules[].output`

Valid only for `kind: map`

Example:

```yaml
output:
    type: proto:eth.erc721.v1.Transfers
```

The value for `type` will always be prefixed by `proto:` followed by a definition you have specified in protobuf definitions, and referenced in the [`protobuf`](manifests.md#protobuf) section.

See [Module Outputs](../concept-and-fundamentals/modules/outputs.md) for details
