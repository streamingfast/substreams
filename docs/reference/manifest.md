# Substreams Manifest

The substream manifest `manifest.yaml` defines the modules that composes the substream. The `manifest.yaml` is used among other things, to infer the dependencies between your module's inputs and outputs. Below is a a reference guide of all fields in the manifest YAML files.&#x20;

## `specVersion`

Example:

```yaml
specVersion: v0.1.0
```

Just make it `v0.1.0`, no questions asked.

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

This field is used to identify your package, and is used to infer the filename when you `substreams pack substreams.yaml`  your package.

* `name` must match this regular expression: `^([a-zA-Z][a-zA-Z0-9_]{0,63})$`, meaning:
* 64 characters maximum
* Separate words with `_`
* Starts with `a-z` or `A-Z`and can contain numbers thereafter

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

The `imports` section imports modules, with their WASM code, all of their (compiled) protobuf definitions and modules definition. The imported modules can be referred to by the _key_ later in the `modules` section.

The _value_ should be a pointer to either a YAML manifest for Substreams Modules (ending in `.yaml`), or an [Package](packages.md) (ending in `.spkg`).

The filename can be an absolute path, or relative (to the location of the `.yaml` file), or be remote if it starts with `http://` or `https://`.

## `protobuf`

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

The `protobuf` section points to the protobuf definitions used by these modules.

The Substreams packager will load files in any of the listed `importPaths`.\
Note that the `imports` section will also affect which `.proto` files end up in your package.

They are packaged with the modules to help clients decode the bytestreams, but are not sent to Substreams server in any way.

Refer to [standard protobuf documentation](https://developers.google.com/protocol-buffers/docs/proto3) for more information about Protocol Buffers.

## `modules`

Examples:

```yaml
  - name: events_extractor
    kind: map
    startBlock: 5000000
    code:
      type: wasm/rust-v1
      file: ../../target/wasm32-unknown-unknown/release/pcs_substreams.wasm
      entrypoint: map_events
    inputs:
      - source: sf.ethereum.type.v1.Block
      - store: myimport:prices
    output:
      type: proto:my.types.v1.Events

  - name: totals
    kind: store
    updatePolicy: sum
    valueType: int64
    code:
      type: wasm/rust-v1
      file: ../../target/wasm32-unknown-unknown/release/pcs_substreams.wasm
      entrypoint: sum_up_totals
    inputs:
      - source: sf.ethereum.type.v1.Block
      - map: events_extractor
```

### `modules[].name`

The identifier for the module, starting with a letter, followed by max 64 characters of `[a-zA-Z0-9_]`. These are the same rules as for `package.name`.

It is the reference identify used on the command line, in inputs and elsewhere to denote this module. It is must be unique per package. Imports get prefixed so imported modules will not clash with the current YAML declaration, even though they share a name.

### `modules[].startBlock`

The start block for the module. The runtime will not process blocks prior to this one for the given module.

The `startBlock` can be inferred by the `inputs` if all the inputs have the same `startBlock`. If some inputs have different `startBlock`, then specifying it is required.

### `modules[].kind`

The type of `module`. There are two types of modules:

* `map`
* `store`

Learn [more about modules here](broken-reference)

### `modules[].updatePolicy`

Valid only for `kind: store`.

Specifies the merge strategy for two contiguous partial stores produced by parallelized operations. See [API Reference](api-reference.md) for details.

Possible values:

* `set` (last key wins merge strategy)
* `set_if_not_exists` (first key wins merge strategy)
* `add` (sum the two keys)
* `min` (min between two keys)
* `max` (you get it)

### `modules[].valueType`

Valid only for `kind: store`.

Specifies the data type of all keys in the `store`, and determines the WASM imports available to the module to write to the store. See [API Reference](api-reference.md) for details.

Possible values:

* `bigfloat`
* `bigint`
* `int64`
* `bytes`
* `string`
* `proto:some.path.to.protobuf.Model`

### `modules[].code`

Specifies the code used to process the data

#### `modules[].code.type`

The type of code, and the implied VM for execution.

At the moment, there is only one VM available, so the value here should be `wasm/rust-v1`

#### `modules[].code.file`

A path pointing to a local compiled [WASM Module](https://webassembly.github.io/spec/core/syntax/modules.html). It can be absolute, or relative to the current `.yaml` file's directory.

This file will be picked up and packaged into an `.spkg` upon invoking `substreams pack`.

#### `modules[].code.entrypoint`

The method to invoke within the [WASM Module](https://webassembly.github.io/spec/core/syntax/modules.html), exported by the Rust code as such:

```rust
#[no_mangle]
pub extern "C" fn my_exported_func(...) {
}
```

A single WASM Module (in WebAssembly parlance) can contain multiple entrypoints.

### `modules[].inputs`

### `modules[].output`

The `output` section is to be defined for `kind: map` modules, and
