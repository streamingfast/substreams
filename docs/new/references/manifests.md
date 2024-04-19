---
description: StreamingFast Substreams manifests reference
---

# Manifests Reference

{% hint style="success" %}
**Tip**: When writing and checking your `substreams.yaml` file, it may help to check your manifest against our [JSON schema](https://json-schema.org/) to ensure there are no problems. JSON schemas can be used in [Jetbrains](https://www.jetbrains.com/help/idea/json.html#ws\_json\_schema\_add\_custom) and [VSCode](https://marketplace.visualstudio.com/items?itemName=redhat.vscode-yaml). Our manifest schema can be seen [here](../../schemas/manifest-schema.json).
{% endhint %}

## Manifests overview

The manifest is the high-level outline for a Substreams module. The manifest file is used for defining properties specific to the current Substreams module and identifying the dependencies between the `inputs` and `outputs` of modules.

This reference documentation **provides a guide for all fields and values** used in a Substreams manifest.

### `specVersion`

Excerpt pulled from the example Substreams manifest.

{% code title="manifest excerpt" %}
```yaml
specVersion: v0.1.0
```
{% endcode %}

Use `v0.1.0` for the `specVersion` field.

### `package`

Excerpt pulled from the example Substreams manifest.

{% code title="manifest excerpt" overflow="wrap" %}
```yaml
package:
  name: module_name_for_project
  version: v0.5.0
  doc: |
    Documentation heading for the package.

    More detailed documentation for the package.
```
{% endcode %}

#### `package.name`

The `package.name` field is used to identify the package.

The `package.name` field infers the filename when the [`pack`](https://substreams.streamingfast.io/reference-and-specs/command-line-interface#pack) command is run by using `substreams.yaml` as a flag for the Substreams package.

The content of the `name` field must match the regular expression: `^([a-zA-Z][a-zA-Z0-9_]{0,63})$`. For consistency, use the `snake_case` naming convention.

The regular expression ruleset translates to the following:

* 64 characters maximum
* Separate words by using `_`
* Starts by using `a-z` or `A-Z` and can contain numbers thereafter

#### `package.version`

The `package.version` field identifies the package for the Substreams module.

{% hint style="info" %}
**Note**: The`package.version` **must respect** [Semantic Versioning, version 2.0](https://semver.org/)
{% endhint %}

#### package.url

The `package.url` field identifies and helps users discover the source of the Substreams package.

#### package.doc

The `package.doc` field is the documentation string of the package. The first line is used by the different UIs as a short-form description.

This field should be written in Markdown format.

### `imports`

The `imports` section allow you to import third-party Substreams packages. It adds local references to modules in those packages, and pull in the WASM code, Protobuf and modules into the current Package.

Relying on imports rather than copying source code from third-party packages allows you to leverage server-side caches, and lower your costs.

Example:

```yaml
imports:
  sol: https://spkg.io/streamingfast/solana-explorer-v0.2.0.spkg
  # or:
  ethereum: substreams-ethereum-v1.0.0.spkg
  token: ../eth-token/substreams.yaml

...

modules:
...
    inputs:
      - map: sol:map_block_without_votes
# replacing:
#    inputs:
#      - source: sf.solana.type.v1.Block
```

Note the `:` separator that signifies to use the imported namespace, as defined under `imports`.

The filename can be absolute or relative or a remote path prefixed by `http://` or `https://`. It can also be an IPFS reference.

### `protobuf`

The `protobuf` section points to the Google Protocol Buffer (protobuf) definitions used by the Rust modules in the Substreams module.

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

The Substreams packager loads files in any of the listed `importPaths`.

{% hint style="info" %}
**Note**: The `imports` section of the manifest also affects which `.proto` files are used in the final Substreams package.
{% endhint %}

Protobufs and modules are packaged together to help Substreams clients decode the incoming streams. Protobufs are not sent to the Substreams server in network requests.

[Learn more about Google Protocol Buffers](https://protobuf.dev/) in the official documentation provided by Google.

### `binaries`

The `binaries` field specifies the WASM binary code to use when executing modules.

The `modules[].binary` field uses a default value of `default`.

```yaml
binaries:
  default:
    type: wasm/rust-v1
    file: ./target/wasm32-unknown-unknown/release/my_package.wasm
  other:
    type: wasm/rust-v1
    file: ./snapshot_of_my_package.wasm
```

{% hint style="warning" %}
**Important**_:_ Defining the `default` binary is required when creating a Substreams manifest.
{% endhint %}

See the [`binary`](manifests.md#module-binary) field under `modules` to see its use.

#### `binaries[name].type`

The type of code and implied virtual machine for execution. There is **only one virtual machine available** that uses a value of: **`wasm/rust-v1`**.

#### `binaries[name].file`

The `binaries[name].file` field references a locally compiled [WASM module](https://webassembly.github.io/spec/core/syntax/modules.html). Paths for the `binaries[name].file` field are absolute or relative to the manifest's directory. The **standard location** of the compiled WASM module is the **root directory** of the Substreams module.

{% hint style="success" %}
**Tip**: The WASM file referenced by the `binary` field is picked up and packaged into an `.spkg` when invoking the [`pack`](https://substreams.streamingfast.io/reference-and-specs/command-line-interface#pack) and [`run`](https://substreams.streamingfast.io/reference-and-specs/command-line-interface#run) commands through the [`substreams` CLI](command-line-interface.md).
{% endhint %}

### network

The `network` field specifies the blockchain where the Substreams will be executed.

```yaml
network: solana
```

or

```yaml
network: ethereum
```

### image

The `image` field specifies the icon displayed for the Substreams package, which is used in the [Substreams Regsitry](https://substreams.dev). The path is relative to the folder where the manifest is.

```yaml
image: ./ethereum-icon.png
```

### sink

The `sink` field specifies the sink you want to use to consume your data (for example, a database or a subgraph).

#### Sink `module`

Specifies the name of the module that emits the data to the sink. For example, `db_out` or `graph_out`.

#### Sink `type`

Specifies the service used to consume the data. For example, `sf.substreams.sink.subgraph.v1.Service` for subgraphs, or `sf.substreams.sink.sql.v1.Service` for databases.

#### Sink `config`

Specifies the configuration specific to every sink. This field is different for every sink.

**Database Config**

```
sink:
  module: db_out
  type: sf.substreams.sink.sql.v1.Service
  config:
    schema: "./schema.sql"
    engine: clickhouse
    postgraphile_frontend:
      enabled: false
    pgweb_frontend:
      enabled: false
    dbt_config:
      enabled: true
      files: "./path/to/folder"
      run_interval_seconds: 300
```

* `schema`: SQL file specifying the schema.
* `engine`: `postgres` or `clickhouse`.
* `postgraphile_frontend.enabled`: enables or disables the Postgraphile portal.
* `pgweb_frontend.enabled`: enables or disables the PGWeb portal.
* `dbt_config`: specifies the configuration of dbt engine.
  * `enabled`: enables or disabled the dbt engine.
  * `files`: path to the dbt models.
  * `run_interval_seconds`: execution intervals in seconds.

**Subgraph Config**

```
sink:
  module: graph_out
  type: sf.substreams.sink.subgraph.v1.Service
  config:
    schema: "./schema.graphql"
    subgraph_yaml: "./subgraph.yaml"
```

* `schema`: path to the GraphQL schema.
* `subgraph_yaml`: path to the Subgraph manifest.

### `modules`

This example shows one map module, named `events_extractor` and one store module, named `totals` :

{% code title="substreams.yaml" %}
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
    doc:
      This module extracts events
      
      Use in such and such situations

  - name: totals
    kind: store
    updatePolicy: add
    valueType: int64
    inputs:
      - source: sf.ethereum.type.v2.Block
      - map: events_extractor
```
{% endcode %}

#### Module `name`

The identifier for the module, prefixed by a letter, followed by a maximum of 64 characters of `[a-zA-Z0-9_]`. The [same rules applied to the `package.name`](manifests.md#package.name) field applies to the module `name`, including the convention to use `snake_case` names.

The module `name` is the reference identifier used on the command line for the `substreams` [`run`](https://substreams.streamingfast.io/reference-and-specs/command-line-interface#run) command. The module `name` is also used in the [`inputs`](manifests.md#modules-.inputs) defined in the Substreams manifest.

The module `name` also corresponds to the **name of the Rust function** invoked on the compiled WASM code upon execution. The module `name` is the same `#[substreams::handlers::map]` as defined in the Rust code. Maps and stores both work in the same fashion.

{% hint style="warning" %}
**Important**_:_ When importing another package, all module names are prefixed by the package's name and a colon. Prefixing ensures there are no name clashes across multiple imported packages and almost any name can be safely used for a module `name`.
{% endhint %}

#### Module `initialBlock`

The initial block for the module is where Substreams begins processing data for a module. The runtime never processes blocks prior to the one for any given module.

If all the inputs have the same `initialBlock`, the field can be omitted and its value is inferred by its dependent [`inputs`](manifests.md#modules-.inputs).

`initialBlock` becomes **mandatory** **when inputs have different values**.

#### Module `kind`

There are two module types for `modules[].kind`:

* `map`
* `store`

#### Module `updatePolicy`

Specifies the merge strategy for two contiguous partial stores produced by parallelized operations.

The values for `modules[].updatePolicy` are defined using specific rules stating:

* `set`, the last key wins the merge strategy
* `set_if_not_exists`, the first key wins the merge strategy
* `append`, concatenates two keys' values
* `add`, sum the two keys' values
* `min`, min between two keys' values
* `max`, max between two keys' values

#### Module `valueType`

{% hint style="success" %}
Tip: The module `updatePolicy` field is only available for modules of `kind: store`.
{% endhint %}

Specifies the data type of all keys in the `store`, and determines what WASM imports are available to the module and are able to write to the `store`.

The values for `modules[].valueTypes` can use various types including:

* `bigfloat`
* `bigint`
* `int64`
* `bytes`
* `string`
* `proto:path.to.custom.protobuf.Model`

{% hint style="success" %}
Tip: The module `valueType` field is only available for modules of `kind: store`.
{% endhint %}

#### Module `binary`

An identifier referring to the [`binaries`](manifests.md#binaries) section of the Substreams manifest.

The `modules[].binary` field overrides which binary is used from the `binaries` declaration section. This means multiple WASM files can be bundled in the Package.

```yaml
modules:
  name: hello
  binary: other
  ...
```

The default value for `binary` is `default`. Therefore, a `default` binary must be defined under [`binaries`](manifests.md#binaries).

#### Module `inputs`

{% code title="substreams.yaml" %}
```yaml
inputs:
    - params: string
    - source: sf.ethereum.type.v2.Block
    - store: my_store
      mode: deltas
    - store: my_store # defaults to mode: get
    - map: my_map
```
{% endcode %}

The `inputs` field is a **list of input structures**. One of three keys is required for every object.

The key types for `inputs` include:

* `source`
* `store,` used to define `mode` keys
* `map`
* `params`

You can find more details about inputs in the [Developer Guide's section about Modules](../developers-guide/modules/types.md).

#### Module `output`

{% code title="substreams.yaml" %}
```yaml
output:
    type: proto:eth.erc721.v1.Transfers
```
{% endcode %}

The value for `type` is always prefixed using `proto:` followed by a definition specified in the protobuf definitions, and referenced in the `protobuf` section of the Substreams manifest.

{% hint style="success" %}
**Tip**: The module `output` field is only available for modules of `kind: map`.
{% endhint %}

#### Module `doc`

This field should contain Markdown documentation of the module. Use it to describe how to use the params, or what to expect from the module.

### `params`

The `params` mapping changes the default values for modules' parameterizable inputs.

```yaml
modules:
  ...
params:
  module_name: "default value"
  "imported:module": "overridden value"
```

You can override those values with the `-p` parameter of `substreams run`.

When rolling out your consuming code -- in this example, Python -- you can use something like:

{% code overflow="wrap" %}
```python
my_mod = [mod for mod in pkg.modules.modules if mod.name == "store_pools"][0]
my_mod.inputs[0].params.value = "myvalue"
```
{% endcode %}

which would be inserted just before starting the stream.
