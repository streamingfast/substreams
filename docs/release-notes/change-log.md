# Change log

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/), and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## Unreleased

### CLI

* Changed `substreams manifest info` -> `substreams info`
* Changed `substreams manifest graph` -> `substreams graph`

## [v0.0.12](https://github.com/streamingfast/substreams/releases/tag/v0.0.12)

### `substreams` server

* Various bug fixes around store and parallel execution.

### `substreams` CLI

* Fix null pointer exception at the end of CLI run in some cases.

* Do log last error when the CLI exit with an error has the error is already printed to the user and it creates a weird behavior.

## [v0.0.11](https://github.com/streamingfast/substreams/releases/tag/v0.0.11)

### `substreams` Docker

* Ensure arguments can be passed to Docker built image.

## [v0.0.10-beta](https://github.com/streamingfast/substreams/releases/tag/v0.0.10-beta)

### `substreams` server

* Various bug fixes around store and parallel execution.
* Fixed logs being repeated on module with inputs that was receiving nothing.

## [v0.0.9-beta](https://github.com/streamingfast/substreams/releases/tag/v0.0.9-beta)

### `substreams` crate

* Added `substreams::hex` wrapper around hex\_literal::hex macro

### `substreams` CLI

* Added `substreams run -o ui|json|jsonl|module-json|module-jsonl`.

### Server

* Fixed a whole bunch of issues, in parallel processing. More stable caching. See chain-specific releases.

## [v0.0.8-beta](https://github.com/streamingfast/substreams/releases/tag/v0.0.8-beta)

* Fixed `substreams` crate usage from tagged version published on crates.io.

## [v0.0.7-beta](https://github.com/streamingfast/substreams/releases/tag/v0.0.7-beta)

* Changed `startBlock` to `initialBlock` in substreams.yaml [manifests](docs/reference-and-specs/manifests.md#modules-.initialblock).
* `code:` is now defined in the `binaries` section of the manifest, instead of in each module. A module can select which binary with the `binary:` field on the Module definition.
* Added `substreams inspect ./substreams.yaml` or `inspect some.spkg` to see what's inside. Requires `protoc` to be installed (which you should have anyway).
* Added command `substreams protogen` that writes a temporary `buf.gen.yaml` and generates Rust structs based on the contents of the provided manifest or package.
*   Added `substreams::handlers` macros to reduce boilerplate when create substream modules.

    `substreams::handlers::map` is used for the handlers corresponding to modules of type `map`. Modules of type `map` should return a `Result` where the error is of type `Error`

    ```rust
    /// Map module example
    #[substreams::handlers::map]
    pub fn map_module_func(blk: eth::Block) -> Result<erc721::Transfers, Error> {
         ...
    }
    ```

    `substreams::handlers::store` is used for the handlers corresponding to modules of type `store`. Modules of type `store` should have no return value.

    ```rust
    /// Map module example
    #[substreams::handlers::store]
    pub fn store_module(transfers: erc721::Transfers, s: store::StoreAddInt64, pairs: store::StoreGet, tokens: store::StoreGet) {
          ...
    }
    ```

## [v0.0.6-beta](https://github.com/streamingfast/substreams/releases/tag/v0.0.6-beta)

* Implemented [packages (see docs)](docs/reference-and-specs/packages.md).
* Added `substreams::Hex` wrapper type to more easily deal with printing and encoding bytes to hexadecimal string.
* Added `substreams::log::info!(...)` and `substreams::log::debug!(...)` supporting formatting arguments (acts like `println!()` macro).
* Added new field `logs_truncated` that can be used to determined if logs were truncated.
* Augmented logs truncation limit to 128 KiB per module per block.
* Updated `substreams run` to properly report module progress error.
* When a module WASM execution error out, progress with failure logs is now returned before closing the substreams connection.
* The API token is not passed anymore if the connection is using plain text option `--plaintext`.
* The `-c` (or `--compact-output`) can be used to print JSON as a single compact line.
* The `--stop-block` flag on `substream run` can be defined as `+1000` to stream from start block + 1000.

## [v0.0.5-beta3](https://github.com/streamingfast/substreams/releases/tag/v0.0.5-beta3)

* Added Dockerfile support.

## [v0.0.5-beta2](https://github.com/streamingfast/substreams/releases/tag/v0.0.5-beta2)

### Client

* Improved defaults for `--proto-path` and `--proto`, using globs.
* WASM file paths in substreams.yaml manifests now resolve relative to the location of the yaml file.
* Added `substreams manifest package` to create .pb packages to simplify querying using other languages. See the python example.
* Added `substreams manifest graph` to show the Mermaid graph alone.
* Improved mermaid graph layout.
* Removed native Go code support for now.

### Server

* Always writes store snapshots, each 10,000 blocks.
* A few tools to manage partial snapshots under `substreams tools`

## [v0.0.5-beta](https://github.com/streamingfast/substreams/releases/tag/v0.0.5-beta)

First chain-agnostic release. THIS IS BETA SOFTWARE. USE AT YOUR OWN RISK. WE PROVIDE NO BACKWARDS COMPATIBILITY GUARANTEES FOR THIS RELEASE.

See https://github.com/streamingfast/substreams for usage docs..

* Removed `local` command. See README.md for instructions on how to run locally now. Build `sfeth` from source for now.
* Changed the `remote` command to `run`.
* Changed `run` command's `--substreams-api-key-envvar` flag to \`\`--substreams-api-token-envvar`, and its default value is changed from` SUBSTREAMS\_API\_KEY`to`SUBSTREAMS\_API\_TOKEN\`. See README.md to learn how to obtain such tokens.
