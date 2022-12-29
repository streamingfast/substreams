---
description: StreamingFast Substreams module handlers
---

# Module handlers

To begin creating the custom module handlers initialize a new Rust project.

```bash
# Creates a barebones rust library
cargo init --lib
```

Update the generated `Cargo.toml` file by using:

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

View the file in the repository:

[https://github.com/streamingfast/substreams-template/blob/develop/Cargo.toml](https://github.com/streamingfast/substreams-template/blob/develop/Cargo.toml)

The Rust code is compiled into [WebAssembly (WASM)](https://webassembly.org/). WASM is a binary instruction format run in a virtual machine. When the Rust code is compiled a `.so` file is generated.

### **Cargo.toml breakdown**

Build the Rust dynamic system library after the `package` by using:

```rust
...

[lib]
crate-type = ["cdylib"]
```

The next definition in the TOML file is for `dependencies`.

Handlers compile down to a WASM module. Explicitly specify the target`asm32-unknown-unknown` by using `[target.wasm32-unknown-unknown.dependencies]`.

#### `ethabi`

The `ethabi` crate is used to decode events from the Application binary interface (ABI) and is required for `substreams-ethereum` ABI capabilities.

#### `hex-literal`

The `hex-literal` crate is used to define bytes from hexadecimal string literal at compile time.

#### `substreams`

The `substreams` crate offers all the basic building blocks for the handlers.

#### `substreams-ethereum`

The `substreams-ethereum` crate offers all the Ethereum constructs including blocks, transactions, eth, and useful `ABI` decoding capabilities.

Because code is being built with WASM output it's necessary to configure Rust to match the correct architecture. Create and add a `rust-toolchain.toml` file at the root of the Substreams directory.

#### Rust toolchain

{% code title="rust-toolchain.toml" %}
```toml
[toolchain]
channel = "1.65"
components = [ "rustfmt" ]
targets = [ "wasm32-unknown-unknown" ]
```
{% endcode %}

View the `rust-toolchain.toml` file in the repository:

[https://github.com/streamingfast/substreams-template/blob/develop/rust-toolchain.toml](https://github.com/streamingfast/substreams-template/blob/develop/rust-toolchain.toml)

Build the code by using:

```rust
cargo build --target wasm32-unknown-unknown --release
```

{% hint style="info" %}
**Note: Rust build target**

* When running `cargo build` the target is set to `wasm32-unknown-unknown`, which is important because it specifies the goal is to generate compiled WebAssembly (WASM) code.
* To avoid having to specify the target `wasm32-unknown-unknown` for every `cargo` command, create a file called `config.toml` in the `.cargo` directory at the root of the Substreams project. The `config.toml` file allows the target to be set automatically for all `cargo` commands.

The content for the `config.toml` file is:

{% code title=".cargo/config.toml" %}
```toml
[build]
target = "wasm32-unknown-unknown"
```
{% endcode %}

With the configuration file in place, the `cargo build` command is now equivalent to `cargo build --target wasm32-unknown-unknown`, so you don't have to specify the target manually every time you build.
{% endhint %}

### ABI generation

The `substreams-ethereum` crate offers an `Abigen` API to generate Rust types from a contract's ABI.

Place the contract ABI JSON file in the Substreams project in an `abi` directory.

{% file src="../../.gitbook/assets/erc721.json" %}

Next, add a Rust build script.

{% hint style="info" %}
**Note: Rust build script**

* Before building a package, Cargo compiles a build script into an executable if it has not already been built. The build script runs as part of the build process responsible for performing a variety of tasks.
* To cause Cargo to compile and run a script before building a package, place a file called `build.rs` in the root of the package.
{% endhint %}

Create a `build.rs` file in the root of the Substreams project by using:

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

View the `build.rs` file in the repository:

[https://github.com/streamingfast/substreams-template/blob/develop/build.rs](https://github.com/streamingfast/substreams-template/blob/develop/build.rs)

Run the build script to generate the ABI directory and files.

```bash
cargo build --target wasm32-unknown-unknown --release
```

Create a `mod.rs` file in the ABI directory, which is created by the Rust build process. The `mod.rs` file is responsible for exporting the generated Rust code.

{% code title="src/abi/mod.rs" %}
```rust
pub mod erc721;
```
{% endcode %}

View the `mod.rs` file in the repository:

[https://github.com/streamingfast/substreams-template/blob/develop/src/abi/mod.rs](https://github.com/streamingfast/substreams-template/blob/develop/src/abi/mod.rs)

The next step is to [write the module handlers](writing-module-handlers.md).
