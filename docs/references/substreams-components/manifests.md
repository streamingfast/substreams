---
description: StreamingFast Substreams manifests reference
---

# Manifests Reference

This reference documentation **provides a guide for all fields and values** used in a Substreams manifest.

{% hint style="success" %}
**Tip**: When writing and checking your `substreams.yaml` file, it may help to check your manifest against our [JSON schema](https://json-schema.org/) to ensure there are no problems. JSON schemas can be used in [Jetbrains](https://www.jetbrains.com/help/idea/json.html#ws\_json\_schema\_add\_custom) and [VSCode](https://marketplace.visualstudio.com/items?itemName=redhat.vscode-yaml). Our manifest schema can be seen [here](../../schemas/manifest-schema.json).
{% endhint %}

## Manifests overview

In simple terms, a Substreams manifest (`substreams.yaml`) is a configuration file (a YAML file) for your Substreams. The manifest file is used for defining properties specific to the current Substreams module and identifying the dependencies between the `inputs` and `outputs` of modules. For example, the following manifest receives a raw Ethereum block as input (`sf.ethereum.type.v2.Block`) and outputs a custom object (`eth.example.MyBlock`).

```yaml
modules:
  - name: map_block
    kind: map
    initialBlock: 12287507
    inputs:
      - source: sf.ethereum.type.v2.Block
    output:
      type: proto:eth.example.MyBlock
```

Among other things, the manifest allows you to define:
- How many modules your Substreams uses, along with their corresponding inputs and outputs.
- The schema(s) (i.e. the data model) your Substreams uses.
- How you will consume the data emitted by your Substreams (SQL, Webhooks...).

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
  url: https://github.com/example/module_name_for_project
  description: One-line description of the package.
  image: ./icon.png
  doc: |
    Documentation heading for the package.

    More detailed documentation for the package.
```
{% endcode %}

#### `package.name`

The `package.name` field is used to identify the package.

The `package.name` field infers the filename when the [`pack`](../cli/command-line-interface.md#pack-deprecated) command is run by using `substreams.yaml` as a flag for the Substreams package.

The content of the `name` field must match the regular expression: `^([a-zA-Z][a-zA-Z0-9_-]{0,63})$`. For consistency, use the `snake_case` naming convention.

The regular expression ruleset translates to the following:

* 64 characters maximum
* Starts with `a-z` or `A-Z`, and can contain numbers, `_`, and `-` thereafter

#### `package.version`

The `package.version` field identifies the package for the Substreams module.

{% hint style="info" %}
**Note**: The`package.version` **must respect** [Semantic Versioning, version 2.0](https://semver.org/)
{% endhint %}

#### `package.url`

The `package.url` field identifies and helps users discover the source of the Substreams package.

#### `package.description`

The `package.description` field is a short, one-line description of the package, shown by `substreams info` and in the [Substreams Registry](https://substreams.dev).

#### `package.doc`

The `package.doc` field is the documentation string of the package. The first line is used by the different UIs as a short-form description.

This field should be written in Markdown format.

#### `package.image`

The `package.image` field is the path to the icon displayed for the package in the [Substreams Registry](https://substreams.dev). The path is relative to the folder that contains the manifest. The file must be a JPEG, WebP, or PNG image of 2 MiB or less.

### `imports`

The `imports` section allows you to import third-party Substreams packages. It adds local references to modules in those packages, and pulls WASM code, Protobuf and modules into the current Package.

Relying on imports rather than copying source code from third-party packages allows you to leverage server-side caches, and lower your costs.

Example:

```yaml
imports:
  sol: https://spkg.io/streamingfast/solana-explorer-v0.2.0.spkg
  # or:
  ethereum: substreams-ethereum-v1.0.0.spkg
  token: ../eth-token/substreams.yaml
  # or, from the Substreams registry:
  erc20: erc20-balances@v1.2.0
  uniswap: streamingfast/uniswap-v4@v0.3.0

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

A registry package is written `<package>@<version>` or, for a package published under an organization, `<org>/<package>@<version>`. The version is required, and `@latest` is refused, so that the same manifest keeps building the same package. When a file or directory named like `<org>/<package>` exists next to the manifest, it is read instead; prefix a path with `./` to make it unambiguous.

### Environment variables

A handful of manifest fields support environment variable expansion using the `$VAR` or `${VAR}` syntax:

- `imports` locations
- `protobuf.importPaths`
- the `foundational-store` module input

This is convenient to author a manifest with a placeholder that is filled in later, for example a hosted-store deployment id:

```yaml
modules:
  - name: my_module
    inputs:
      - foundational-store: $DEPLOYMENT_ID
```

Expansion happens when the manifest is packed or loaded. If a referenced variable is not set in the environment, the operation fails with an error rather than substituting an empty value.

{% hint style="warning" %}
**Caveat**: Environment variable expansion is a convenience that applies only while reading the manifest. The generated `.spkg` always embeds the resolved, hard-coded value — never the `$VAR` reference. Re-packaging in a different environment is what changes the resolved value.
{% endhint %}

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

#### `protobuf.excludePaths`

The `excludePaths` field lists `.proto` files or directories that `substreams build` and `substreams protogen` skip when generating Rust code, for example `sf/substreams` or `google`. Use it for types that come from a crate you already depend on. The `--exclude-paths` flag of `substreams protogen` takes precedence over this field.

```yaml
protobuf:
  files:
    - mydata.proto
  importPaths:
    - ./proto
  excludePaths:
    - sf/substreams
    - google
```

{% hint style="info" %}
**Note**: The `imports` section of the manifest also affects which `.proto` files are used in the final Substreams package.
{% endhint %}

Protobufs and modules are packaged together to help Substreams clients decode the incoming streams. Protobufs are not sent to the Substreams server in network requests.

[Learn more about Google Protocol Buffers](https://protobuf.dev/) in the official documentation provided by Google.

#### `protobuf.descriptorSets`

The `descriptorSets` field allows you to import precompiled Protocol Buffer definitions from the [Buf Schema Registry (BSR)](https://buf.build). This is useful when you want to consume protobuf types from external packages without copying `.proto` files into your project.

Descriptor sets are precompiled binary representations of protobuf schemas that can be directly loaded by Substreams, enabling efficient type resolution and validation.

{% hint style="info" %}
**Note**: Using `descriptorSets` is an alternative to specifying `.proto` files via the `files` and `importPaths` fields. You can use both approaches in the same manifest if needed.
{% endhint %}

**Format 1: Separate version field**

{% code title="substreams.yaml" %}
```yaml
protobuf:
  descriptorSets:
    - module: buf.build/streamingfast/substreams-sink-sql
      version: v0.1.0
```
{% endcode %}

**Format 2: Inline version with @ notation**

{% code title="substreams.yaml" %}
```yaml
protobuf:
  descriptorSets:
    - module: buf.build/streamingfast/substreams-sink-sql@v0.1.0
```
{% endcode %}

**Available Fields:**

* `module` (required): The full path to the Buf module in the format `buf.build/organization/repository`. Can optionally include the version using `@version` notation.
* `version` (optional): Either a valid semantic version (e.g., `v0.1.0`, `v1.2.3`) or `latest`.
* `symbols` (optional): An array of specific protobuf symbols to import from the descriptor set. If omitted, all types from the descriptor set are available.
* `localPath` (optional): Local filesystem path where the descriptor set should be cached or stored.

**Complete Example with All Fields:**

{% code title="substreams.yaml" %}
```yaml
protobuf:
  descriptorSets:
    - module: buf.build/streamingfast/substreams-sink-sql
      version: v1.0.0
      symbols:
        - sf.substreams.sink.sql.v1.Service
        - sf.substreams.sink.sql.v1.Table
      localPath: ./proto-cache/sink-sql.binpb
    - module: buf.build/streamingfast/substreams-entity-change@v1.3.0
      symbols:
        - sf.substreams.entity.v1.EntityChanges
```
{% endcode %}

**Version Validation Rules:**

{% hint style="warning" %}
**Important**: When using inline `@version` notation:

* Versions **must** be valid semantic versions (e.g., `v1.0.0`, `v0.2.5`)
* When using inline `@version` notation:
  * Only semantic versions are allowed (e.g., `module@v1.0.0`)
  * `@latest` is **not allowed**, use `version: latest` as a separate field or omit the version
* You **cannot** specify the version both inline (with `@`) and as a separate field, choose one format
* To use the latest version, either:
  * Omit the version field entirely
  * Use `version: latest` as a separate field
{% endhint %}
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

The type of code and implied virtual machine for execution. The supported values are:

* `wasm/rust-v1`: a Rust module compiled to `wasm32-unknown-unknown`. This is the standard type.
* `wasip1/tinygo-v1` (alpha): a Go module compiled with [TinyGo](https://tinygo.org) to the `wasi` target. `substreams build` compiles it with `tinygo build -o main.wasm -target wasi -gc leaking -scheduler none .`.

A type can take runtime extensions, appended after a `+` and separated by commas. The only available extension is `wasm-bindgen-shims`, for `wasm/rust-v1`:

```yaml
binaries:
  default:
    type: wasm/rust-v1+wasm-bindgen-shims
    file: ./target/wasm32-unknown-unknown/release/my_package.wasm
```

Enable `wasm-bindgen-shims` when your Rust code depends on crates such as `solana_program` or `alloy`, which add [wasm-bindgen](https://wasm-bindgen.github.io/wasm-bindgen/) imports to the compiled module. The shims satisfy those imports so the module loads. If the code calls one of those imports at run time, the module fails.

#### `binaries[name].file`

The `binaries[name].file` field references a locally compiled [WASM module](https://webassembly.github.io/spec/core/syntax/modules.html). Paths for the `binaries[name].file` field are absolute or relative to the manifest's directory. The **standard location** of the compiled WASM module is the **root directory** of the Substreams module.

{% hint style="success" %}
**Tip**: The WASM file referenced by the `binary` field is picked up and packaged into an `.spkg` when invoking the [`pack`](../cli/command-line-interface.md#pack-deprecated) and [`run`](../cli/command-line-interface.md#run) commands through the [`substreams` CLI](../cli/command-line-interface.md).
{% endhint %}

#### `binaries[name].build`

The `binaries[name].build` field overrides the command that `substreams build` runs to compile the binary. The command runs in the manifest's directory and is split on spaces, without shell interpretation, so pipes, `&&`, and quoted arguments are not supported. When the field is omitted, `wasm/rust-v1` binaries are built with `cargo build --target wasm32-unknown-unknown --release`.

```yaml
binaries:
  default:
    type: wasm/rust-v1
    file: ./target/wasm32-unknown-unknown/release/my_package.wasm
    build: cargo build --target wasm32-unknown-unknown --release --features my_feature
```

### `network`

The `network` field specifies the default network for this Substreams, for example `mainnet` or `solana-mainnet`. Clients use it to pick an endpoint, and it selects which entry of [`networks`](manifests.md#networks) applies by default.

```yaml
network: mainnet
```

### `sink`

The `sink` field specifies the sink you want to use to consume your data (for example, a database).

#### Sink `module`

Specifies the name of the module that emits the data to the sink. For example, `db_out` or `graph_out`.

#### Sink `type`

Specifies the service used to consume the data. For example, `sf.substreams.sink.sql.v1.Service` for databases.

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

The identifier for the module, prefixed by a letter, followed by a maximum of 63 characters of `[a-zA-Z0-9_-]`. The [same rules applied to the `package.name`](manifests.md#package.name) field applies to the module `name`, including the convention to use `snake_case` names. Because the name is also the Rust function name, a module that runs its own code cannot use `-`.

The module `name` is the reference identifier used on the command line for the `substreams` [`run`](../../references/cli/command-line-interface.md#run) command. The module `name` is also used in the [`inputs`](./modules/inputs.md) defined in the Substreams manifest.

The module `name` also corresponds to the **name of the Rust function** invoked on the compiled WASM code upon execution. The module `name` is the same `#[substreams::handlers::map]` as defined in the Rust code. Maps and stores both work in the same fashion.

{% hint style="warning" %}
**Important**_:_ When importing another package, all module names are prefixed by the package's name and a colon. Prefixing ensures there are no name clashes across multiple imported packages and almost any name can be safely used for a module `name`.
{% endhint %}

#### Module `initialBlock`

The initial block for the module is where Substreams begins processing data for a module. The runtime never processes blocks prior to the one for any given module.

If all the inputs have the same `initialBlock`, the field can be omitted and its value is inferred by its dependent [`inputs`](manifests.md#modules-.inputs).

`initialBlock` becomes **mandatory** **when inputs have different values**.

#### Module `kind`

There are three module types for `modules[].kind`:

* `map`
* `store`
* `blockIndex`, a map whose output type is `proto:sf.substreams.index.v1.Keys`, used by other modules' [`blockFilter`](manifests.md#module-blockfilter). It cannot take `params` inputs or have a `blockFilter` itself. See [Indexes](./modules/indexes.md).

`kind` is required, unless the module sets [`use`](manifests.md#module-use), in which case it must be omitted.

#### Module `updatePolicy`

Specifies the merge strategy for two contiguous partial stores produced by parallelized operations.

The values for `modules[].updatePolicy` are defined using specific rules stating:

* `set`, the last key wins the merge strategy
* `set_if_not_exists`, the first key wins the merge strategy
* `append`, concatenates two keys' values
* `add`, sum the two keys' values
* `min`, min between two keys' values
* `max`, max between two keys' values
* `set_sum`, either `set` the value or `sum` the two keys' values

{% hint style="success" %}
Tip: The module `updatePolicy` field is only available for modules of `kind: store`.
{% endhint %}

#### Module `valueType`

Specifies the data type of all keys in the `store`, and determines what WASM imports are available to the module and are able to write to the `store`.

The values for `modules[].valueTypes` can use various types including:

* `bigdecimal`
* `bigfloat` (kept for backward compatibility, prefer `bigdecimal`)
* `bigint`
* `int64`
* `float64`
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
  - name: hello
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
    - foundational-store: my_foundational_store@v1.0.0
```
{% endcode %}

The `inputs` field is a **list of input structures**. Each object has exactly one of these keys:

* `source`
* `store`, with an optional `mode` of `get` (default) or `deltas`
* `map`
* `params`
* `foundational-store`, see the [Foundational Stores reference](../foundational-store-reference.md)

You can find more details about inputs in the [Developer Guide's section about Modules](../substreams-components/modules/modules.md).

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

#### Module `blockFilter`

The `blockFilter` field makes the module skip blocks, based on the keys emitted by a module of `kind: blockIndex`. A skipped block is not processed by the module, which saves the cost of decoding blocks that hold nothing of interest.

{% code title="substreams.yaml" %}
```yaml
  - name: filtered_events
    kind: map
    blockFilter:
      module: index_events
      query:
        string: evt_addr:0xa0b86a33e6776e1b1c4b0b8b8b8b8b8b8b8b8b8b
    inputs:
      - map: all_events
    output:
      type: proto:my.types.v1.Events
```
{% endcode %}

* `module`: name of the `blockIndex` module to read keys from. Its `initialBlock` cannot be greater than the `initialBlock` of the filtered module.
* `query.string`: the query to match against the block keys, for example `evt_addr:0xa0b8 || evt_addr:0x1234`. The `&&` and `||` operators and parentheses are supported.
* `query.params: true`: use the value of the module's `params` input as the query, so it can be changed with `-p` at run time. The module must have a `params` input.

Set exactly one of `query.string` or `query.params`. Setting `blockFilter: {}` means no filter, which is useful to remove the filter inherited through [`use`](manifests.md#module-use).

See [Indexes](./modules/indexes.md) for a complete example.

#### Module `use`

The `use` field declares a module that runs the code of another module, already defined in this manifest or in an imported package. Use it to run the same module more than once with different `params`, `initialBlock`, or `inputs`, without writing any code. You can also use it to keep running a module from a previous version of your package: the module keeps its previous code, so its hash and its cached data stay the same. See [Updating your Substreams](../../how-to-guides/updating-your-substreams.md).

{% code title="substreams.yaml" %}
```yaml
imports:
  ethereum_common: ethereum_common@v0.3.3

modules:
  - name: usdc_logs
    use: ethereum_common:filtered_logs
    initialBlock: 6082465

  - name: usdt_logs
    use: ethereum_common:filtered_logs
    initialBlock: 4634748

params:
  usdc_logs: "address=0xa0b86991c6218b36c1d19d4a2e9eb0ce3606eb48"
  usdt_logs: "address=0xdac17f958d2ee523a2206206994597c13d831ec7"
```
{% endcode %}

The module copies the following from the used module:

* `kind`, including `updatePolicy` and `valueType` for a store
* `output`
* the binary and the Rust function it calls
* `inputs`, when the module has no `inputs` of its own. A `params` input keeps the used module's default value, which you can override under [`params`](manifests.md#params).
* `blockFilter`, when the module has no `blockFilter` of its own. Set `blockFilter: {}` to run without the inherited filter.

`initialBlock` is not copied: set it on the module, or let it be derived from the module's inputs, as for any other module. `doc` is not copied either.

A module that sets `use` cannot set `kind`, `output`, `updatePolicy`, `valueType`, or `binary`.

When you set `inputs`, they must match the inputs of the used module one by one, in the same order:

* the same number of inputs
* a `source` input has the same type
* a `store` input has the same `mode`
* a `map` input points to a module with the same output type
* a `params` input stays a `params` input

Other modules, `params`, `networks`, and the [`sink`](manifests.md#sink) refer to the new module by its own `name`.

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

Params that are defined under `networks` do not need to be repeated here (their value will be overwritten)

### `networks`

The `networks` allows specifying per-network `params` and `initialBlock` for each module:

```yaml
networks:
  mainnet:
    initialBlock:
      mod1: 200
      lib:mod1: 400
    params:
      mod2: "addr=0x1234"
  sepolia:
    [...]
```

You can override values for modules imported from other .spkg.

Every local module specified under `networks` must have a value for **each network**
