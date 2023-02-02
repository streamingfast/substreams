---
description: StreamingFast Substreams module handlers
---

# Module handlers

## Module handlers overview

To begin creating the custom module handlers initialize a new Rust project by using the `cargo` `init` command.

```bash
# Creates a empty Rust project suitable for WASM compilation
cargo init --lib
```

Update the generated [`Cargo.toml`](https://github.com/streamingfast/substreams-template/blob/develop/Cargo.toml) file by using:

{% code title="Cargo.toml" overflow="wrap" lineNumbers="true" %}
```rust
[package]
name = "substreams-template"
version = "0.1.0"
description = "Substream template demo project"
edition = "2021"
repository = "https://github.com/streamingfast/substreams-template"

[lib]
name = "substreams"
crate-type = ["cdylib"]

[dependencies]
ethabi = "17"
hex-literal = "0.3.4"
prost = "0.11"
# Use latest from https://crates.io/crates/substreams
substreams = "0.5"
# Use latest from https://crates.io/crates/substreams-ethereum
substreams-ethereum = "0.9"

# Required so ethabi > ethereum-types build correctly under wasm32-unknown-unknown
[target.wasm32-unknown-unknown.dependencies]
getrandom = { version = "0.2", features = ["custom"] }

[build-dependencies]
anyhow = "1"
substreams-ethereum = "0.8"

[profile.release]
lto = true
opt-level = 's'
strip = "debuginfo"
```
{% endcode %}

View the [`Cargo.toml`](https://github.com/streamingfast/substreams-template/blob/develop/Cargo.toml) file in the repository.

You compile the Rust code into [WebAssembly (WASM)](https://webassembly.org/), a binary instruction format that runs in a virtual machine. The compilation process generates a .so file.

### **`Cargo.toml` configuration file breakdown**

Build the Rust dynamic system library after the `package` by using:

{% code title="Cargo.toml excerpt" %}
```toml
...

[lib]
crate-type = ["cdylib"]
```
{% endcode %}

The next definition in the [`Cargo.toml`](https://github.com/streamingfast/substreams-template/blob/develop/Cargo.toml) configuration file is for `dependencies`.

{% hint style="info" %}
**Note**: Module handlers compile down to a WASM module. Explicitly specify the target`asm32-unknown-unknown` by using `[target.wasm32-unknown-unknown.dependencies]`.
{% endhint %}

#### `ethabi`

The [`ethabi` crate ](https://crates.io/crates/ethabi)is used to decode events from the application binary interface (ABI) and is required for `substreams-ethereum` ABI capabilities.

#### `hex-literal`

The [`hex-literal` crate ](https://crates.io/crates/hex-literal)is used to define bytes from hexadecimal string literals at compile time.

#### `substreams`

The [`substreams` crate](https://docs.rs/substreams/latest/substreams/) offers all the basic building blocks for the module handlers.

#### `substreams-ethereum`

The [`substreams-ethereum` crate](https://crates.io/crates/substreams-ethereum-core) offers all the Ethereum constructs including blocks, transactions, eth, and useful ABI decoding capabilities.

Because code is being built by WASM output it's necessary to configure Rust to match the correct architecture. Create and add a [`rust-toolchain.toml`](https://github.com/streamingfast/substreams-template/blob/develop/rust-toolchain.toml) configuration file at the root of your Substreams directory.

### Rust toolchain

{% code title="rust-toolchain.toml" overflow="wrap" lineNumbers="true" %}
```toml
[toolchain]
channel = "1.65"
components = [ "rustfmt" ]
targets = [ "wasm32-unknown-unknown" ]
```
{% endcode %}

View the [`rust-toolchain.toml`](https://github.com/streamingfast/substreams-template/blob/develop/rust-toolchain.toml) file in the repository.

Build the code by using:

```bash
cargo build --target wasm32-unknown-unknown --release
```

### **Rust build target**

When running `cargo build` the target is set to `wasm32-unknown-unknown`, which is important because it specifies the goal is to generate compiled WASM code.

To avoid having to specify the target `wasm32-unknown-unknown` for every `cargo` command, create a `config.toml` configuration file in the `.cargo` directory at the root of the Substreams project. The `config.toml` configuration file allows the target to be set automatically for all `cargo` commands.

The content for the `config.toml` configuration file is:

{% code title=".cargo/config.toml" %}
```toml
[build]
target = "wasm32-unknown-unknown"
```
{% endcode %}

The `config.toml` configuration file updates the default `cargo build` command to `cargo build --target wasm32-unknown-unknown` eliminating the need to specify the target manually every time you build.

### ABI generation

The [`substreams-ethereum` crate](https://crates.io/crates/substreams-ethereum) offers an [`Abigen`](https://docs.rs/substreams-ethereum-abigen/latest/substreams_ethereum_abigen/build/struct.Abigen.html) API to generate Rust types from a smart contract's ABI.

Place the contract's [ABI JSON file](../../.gitbook/assets/erc721.json) in the Substreams project in the `abi` directory.

### **Rust build script**

Before building a package, Cargo compiles a build script into an executable if it has not already been built. The build script runs as part of the build process responsible for performing a variety of tasks.

To cause Cargo to compile and run a script before building a package, place a file called `build.rs` in the root of the package.

Create a [`build.rs`](https://github.com/streamingfast/substreams-template/blob/develop/build.rs) build script file in the root of the Substreams project by using:

{% code title="build.rs" overflow="wrap" lineNumbers="true" %}
```rust
use anyhow::{Ok, Result};
use substreams_ethereum::Abigen;

fn main() -> Result<(), anyhow::Error> {
    Abigen::new("ERC721", "abi/erc721.json")?
        .generate()?
        .write_to_file("src/abi/erc721.rs")?;

    Ok(())
}
```
{% endcode %}

View the [`build.rs`](https://github.com/streamingfast/substreams-template/blob/develop/build.rs) file in the repository.

Run the build script to generate the ABI directory and files.

```bash
cargo build --target wasm32-unknown-unknown --release
```

Create a [`mod.rs`](https://github.com/streamingfast/substreams-template/blob/develop/src/abi/mod.rs) export file in the ABI directory, which is created by the Rust build process. The [`mod.rs`](https://github.com/streamingfast/substreams-template/blob/develop/src/abi/mod.rs) export file is responsible for exporting the generated Rust code.

{% code title="src/abi/mod.rs" lineNumbers="true" %}
```rust
pub mod erc721;
```
{% endcode %}

View the [`mod.rs`](https://github.com/streamingfast/substreams-template/blob/develop/src/abi/mod.rs) file in the repository.

You're now ready to [write the module handlers](writing-module-handlers.md).
