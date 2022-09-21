---
description: StreamingFast Substreams module handlers
---

# Module Handlers

Now that we generated our `Protobuf` Rust code, let's initiate our Rust project and generate some code to set us up to write our handlers:

```bash
# This is to create a barebones rust project
cargo init
# Since we are building a library we need to rename the newly generated main.rs
mv ./src/main.rs ./src/lib.rs
```

Let's edit the newly created `Cargo.toml` file to look like this:

{% code title="Cargo.toml" %}
```rust
[package]
name = "substreams-example"
version = "0.1.0"
description = "Substream template demo project"
edition = "2021"
repository = "https://github.com/streamingfast/substreams-template"

[lib]
crate-type = ["cdylib"]

[target.wasm32-unknown-unknown.dependencies]
ethabi = "17.0"
hex-literal = "0.3.4"
prost = { version = "0.11.0" }
# Use latest from https://crates.io/crates/substreams
substreams = { version = "0.0.20" }
# Use latest from https://crates.io/crates/substreams-ethereum
substreams-ethereum = { version = "0.2.1" }

# Required so that ethabi > ethereum-types build correctly under wasm32-unknown-unknown
getrandom = { version = "0.2", features = ["custom"] }

[build-dependencies]
anyhow = "1"
substreams-ethereum = { version = "0.2.1" }

[profile.release]
lto = true
opt-level = 's'
strip = "debuginfo"
```
{% endcode %}

Let's go through the important changes. Our Rust code will be compiled in [`wasm`](https://webassembly.org/). Think of `wasm` code as a binary instruction format that can be run in a virtual machine. When your Rust code is compiled, it will generate a `.so` file.

**Let's break down our `Cargo.toml` file**

Since we are building a Rust dynamic system library, after the `package`, we first need to specify:

```rust
...

[lib]
crate-type = ["cdylib"]
```

We then need to specify our `dependencies`. We specify explicitly the `wasm32-unknown-unknown` (using `[target.wasm32-unknown-unknown.dependencies]`) target since our handlers compile down to a WASM module:

* `ethabi`: This crate will be used to decode events from your ABI, required for `substreams-ethereum` ABI functionalities.
* `hex-literal`: This crate will be used to define bytes from hexadecimal string literal at compile time.
* `substreams`: This crate offers all the basic building blocks for your handlers.
* `substreams-ethereum`: This crate offers all the Ethereum constructs (blocks, transactions, eth) as well as useful `ABI` decoding capabilities.

Since we are building our code into `wasm,` we need to configure Rust to target the correct architecture. Add this file at the root of your Substreams directory

{% code title="rust-toolchain.toml" %}
```toml
[toolchain]
channel = "1.60.0"
components = [ "rustfmt" ]
targets = [ "wasm32-unknown-unknown" ]
```
{% endcode %}

We can now build our code

```rust
cargo build --target wasm32-unknown-unknown --release
```

{% hint style="info" %}
**Rust Build Target**

Notice that when we run `cargo build` we specify the `target` to be `wasm32-unknown-unknown.` This is important, since the goal is to generate compiled `wasm` code. You can avoid having to manually specify `--target wasm32-unknown-unknown` for each `cargo` command by creating a file named `config.toml` under folder `.cargo` at the root of your project with the following content:

{% code title=".cargo/config.toml" %}
```toml
[build]
target = "wasm32-unknown-unknown"
```
{% endcode %}

With this config file, `cargo build` is now equivalent to `cargo build --target wasm32-unknown-unknown`.
{% endhint %}

### ABI Generation

In order to make it easy and type-safe to work with smart contracts, the `substreams-ethereum` crate offers an `Abigen` API to generate Rust types from a contract's ABI.

We will first insert our contract ABI json file in our projects under an `abi` folder.

{% file src="../.gitbook/assets/erc721.json" %}

Now that we have an ABI in our project, let's add a Rust build script.

{% hint style="info" %}
**Rust Build Script**

Just before a package is built, Cargo will compile a build script into an executable (if it has not already been built). It will then run the script, which may perform any number of tasks.

Placing a file named `build.rs` in the root of a package will cause Cargo to compile that script and execute it just before building the package.
{% endhint %}

We will create a `build.rs` file in the root of our Substreams project:

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

We will run the build script by building the project

```bash
cargo build --target wasm32-unknown-unknown --release
```

You should now have a generated ABI folder `src/abi.` Next, we will create a `mod.rs` file in that folder to export the generated Rust code

{% code title="src/abi/mod.rs" %}
```rust
pub mod erc721;
```
{% endcode %}

We can now write our Rust handlers!
