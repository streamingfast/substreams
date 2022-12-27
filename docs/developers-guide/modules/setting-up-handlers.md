---
description: StreamingFast Substreams module handlers
---

# Module handlers

To begin creating the custom module handlers initialize a new Rust project.

```bash
# This is to create a barebones rust library
cargo init --lib
```

Update the generated `Cargo.toml` file to match the following.

{% code title="Cargo.toml" %}
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
substreams-ethereum = "0.8"

# Required so that ethabi > ethereum-types build correctly under wasm32-unknown-unknown
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

View the file in the repo:

[https://github.com/streamingfast/substreams-template/blob/develop/Cargo.toml](https://github.com/streamingfast/substreams-template/blob/develop/Cargo.toml)

The Rust code will be compiled into [WebAssembly (WASM)](https://webassembly.org/). WASM is a binary instruction format that can be run in a virtual machine. When the Rust code is compiled a `.so` file is generated.

### **Cargo.toml breakdown**

To build the Rust dynamic system library, after the `package`, specify the following.

```rust
...

[lib]
crate-type = ["cdylib"]
```

The next definition in the TOML file is for `dependencies`.

Handlers compile down to a WASM module. Explicitly specify the target`asm32-unknown-unknown` using `[target.wasm32-unknown-unknown.dependencies]`.

#### `ethabi`

The `ethabi` crate will be used to decode events from the Application binary interface (ABI) and is required for `substreams-ethereum` ABI functionality.

#### `hex-literal`

The `hex-literal` crate will be used to define bytes from hexadecimal string literal at compile time.

#### `substreams`

The `substreams` crate offers all the basic building blocks for the handlers.

#### `substreams-ethereum`

The `substreams-ethereum` crate offers all the Ethereum constructs including blocks, transactions, eth, and useful `ABI` decoding capabilities.

Because code is being built with WASM output it's necessary to configure Rust to target the correct architecture. Create and add a rust-toolchain.toml file at the root of the Substreams directory.

#### Rust toolchain

{% code title="rust-toolchain.toml" %}
```toml
[toolchain]
channel = "1.65"
components = [ "rustfmt" ]
targets = [ "wasm32-unknown-unknown" ]
```
{% endcode %}

View this file in the repo:

[https://github.com/streamingfast/substreams-template/blob/develop/rust-toolchain.toml](https://github.com/streamingfast/substreams-template/blob/develop/rust-toolchain.toml)

The code can now be built using:

```rust
cargo build --target wasm32-unknown-unknown --release
```

{% hint style="info" %}
**Rust build target**

Notice when `cargo build` is run the `target` is `wasm32-unknown-unknown.` This specification is important since the goal is to generate compiled WASM code.

To avoid manually specifying `target wasm32-unknown-unknown` for each `cargo` command, create a file named `config.toml` in the `.cargo` directory at the root of the Substreams project.

The content for the file should be:

{% code title=".cargo/config.toml" %}
```toml
[build]
target = "wasm32-unknown-unknown"
```
{% endcode %}

With the configuration file in place the `cargo build` command is now equivalent to `cargo build --target wasm32-unknown-unknown`.
{% endhint %}

### ABI generation

The `substreams-ethereum` crate offers an `Abigen` API to generate Rust types from a contract's ABI.

Insert the contract ABI JSON file in the Substreams project in an `abi` directory.

{% file src="../../.gitbook/assets/erc721.json" %}

Next, add a Rust build script.

{% hint style="info" %}
**Rust Build Script**

Just before a package is built, Cargo will compile a build script into an executable (if it has not already been built). It will then run the script, which may perform any number of tasks.

Placing a file named `build.rs` in the root of a package will cause Cargo to compile that script and execute it just before building the package.
{% endhint %}

Create a `build.rs` file in the root of the Substreams project using the following code.

{% code title="build.rs" %}
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

View this file in the repo by visiting the following link.

[https://github.com/streamingfast/substreams-template/blob/develop/build.rs](https://github.com/streamingfast/substreams-template/blob/develop/build.rs)

Run the build script to generate the ABI directory and files.

```bash
cargo build --target wasm32-unknown-unknown --release
```

Next, create a `mod.rs` file in the abi directory (that was created by the Rust build process) to export the generated Rust code.

{% code title="src/abi/mod.rs" %}
```rust
pub mod erc721;
```
{% endcode %}

View this file in the repo by visiting the following link.

[https://github.com/streamingfast/substreams-template/blob/develop/src/abi/mod.rs](https://github.com/streamingfast/substreams-template/blob/develop/src/abi/mod.rs)

The next step in the Substreams setup process is to write the actual module handlers themselves.
