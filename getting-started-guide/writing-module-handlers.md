# Writing Module Handlers

Now that we generated our Protobuf Rust code, let's initiate our Rust project and write our handlers

```bash
# This is create a barebones rust project
cargo init
# Since we are building a library we need to rename the newly generated main.rs
mv ./src/main.rs ./src/lib.rs
```

Lets edit the newly created `Cargo.toml` file to look like this:

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

[dependencies]
ethabi = "17.0"
hex-literal = "0.3.4"
prost = { version = "0.10.1" }
substreams= { git = "https://github.com/streamingfast/substreams", branch="develop" }
substreams-ethereum = { git = "https://github.com/streamingfast/substreams-ethereum", branch="develop" }

# Required so that ethabi > ethereum-types build correctly under wasm32-unknown-unknown
getrandom = { version = "0.2", features = ["custom"] }


[build-dependencies]
anyhow = "1"
substreams-ethereum = { git = "https://github.com/streamingfast/substreams-ethereum", branch="develop" }

[profile.release]
lto = true
opt-level = 's'
strip = "debuginfo"
```
{% endcode %}

Let's go through the important changes. Our Rust code will be compiled in [`wasm`](https://webassembly.org/). Think of `wasm` code as a binary instruction format that can be run in a virtual machine. When your Rust code is compiled it will generate a `.so` file.&#x20;

**Let's break down the file**

Since we are building a Rust dynamic system library, after the `package`, we first need to specify:

```rust
...

[lib]
crate-type = ["cdylib"]
```

We then need to specify our `dependencies:`

* `ethabi`: This crate will be used to decode events from your ABI
* `hex-literal`: This crate will be used to manipulate Hexadecimal values
* `substreams`: This crate offers all the basic building blocks for your handlers
* `substreams-ethereum`: This crate offers all the Ethereum constructs (blocks, transactions, eth) as well as useful `ABI` decoding capabilities

Since we are building our building our code into `wasm` we need to configure Rust to target the correct architecture. Add this file at the root of our Substreams director

```toml
[toolchain]
channel = "1.60.0"
components = [ "rustfmt" ]
targets = [ "wasm32-unknown-unknown" 
```

We can now build our code

```rust
cargo build --target wasm32-unknown-unknown --release
```

{% hint style="info" %}
**Rust Build Target**

Notice that when we run `cargo build` we specify the `target` to be `wasm32-unknown-unknown` this is important, since the goal is to generate compiled `wasm` code.
{% endhint %}

### ABI Generation

In order to make it easy and type-safe to work with smart contracts, the `substreams-ethereum` crate offers an `Abigen` API to generate Rust types from a contracts ABI.&#x20;

We will first insert our contract ABI json file in our projects under an `abi` folder

{% code title="abi/erc721.json" %}
```json
[
	{
		"anonymous": false,
		"inputs": [
			{
				"indexed": true,
				"internalType": "address",
				"name": "owner",
				"type": "address"
			},
			{
				"indexed": true,
				"internalType": "address",
				"name": "approved",
				"type": "address"
			},
			{
				"indexed": true,
				"internalType": "uint256",
				"name": "tokenId",
				"type": "uint256"
			}
		],
		"name": "Approval",
		"type": "event"
	},
	{
		"anonymous": false,
		"inputs": [
			{
				"indexed": true,
				"internalType": "address",
				"name": "owner",
				"type": "address"
			},
			{
				"indexed": true,
				"internalType": "address",
				"name": "operator",
				"type": "address"
			},
			{
				"indexed": false,
				"internalType": "bool",
				"name": "approved",
				"type": "bool"
			}
		],
		"name": "ApprovalForAll",
		"type": "event"
	},
	{
		"anonymous": false,
		"inputs": [
			{
				"indexed": true,
				"internalType": "address",
				"name": "from",
				"type": "address"
			},
			{
				"indexed": true,
				"internalType": "address",
				"name": "to",
				"type": "address"
			},
			{
				"indexed": true,
				"internalType": "uint256",
				"name": "tokenId",
				"type": "uint256"
			}
		],
		"name": "Transfer",
		"type": "event"
	},
	{
		"inputs": [
			{
				"internalType": "address",
				"name": "to",
				"type": "address"
			},
			{
				"internalType": "uint256",
				"name": "tokenId",
				"type": "uint256"
			}
		],
		"name": "approve",
		"outputs": [],
		"stateMutability": "nonpayable",
		"type": "function"
	},
	{
		"inputs": [
			{
				"internalType": "address",
				"name": "owner",
				"type": "address"
			}
		],
		"name": "balanceOf",
		"outputs": [
			{
				"internalType": "uint256",
				"name": "balance",
				"type": "uint256"
			}
		],
		"stateMutability": "view",
		"type": "function"
	},
	{
		"inputs": [
			{
				"internalType": "uint256",
				"name": "tokenId",
				"type": "uint256"
			}
		],
		"name": "getApproved",
		"outputs": [
			{
				"internalType": "address",
				"name": "operator",
				"type": "address"
			}
		],
		"stateMutability": "view",
		"type": "function"
	},
	{
		"inputs": [
			{
				"internalType": "address",
				"name": "owner",
				"type": "address"
			},
			{
				"internalType": "address",
				"name": "operator",
				"type": "address"
			}
		],
		"name": "isApprovedForAll",
		"outputs": [
			{
				"internalType": "bool",
				"name": "",
				"type": "bool"
			}
		],
		"stateMutability": "view",
		"type": "function"
	},
	{
		"inputs": [],
		"name": "name",
		"outputs": [
			{
				"internalType": "string",
				"name": "",
				"type": "string"
			}
		],
		"stateMutability": "view",
		"type": "function"
	},
	{
		"inputs": [
			{
				"internalType": "uint256",
				"name": "tokenId",
				"type": "uint256"
			}
		],
		"name": "ownerOf",
		"outputs": [
			{
				"internalType": "address",
				"name": "owner",
				"type": "address"
			}
		],
		"stateMutability": "view",
		"type": "function"
	},
	{
		"inputs": [
			{
				"internalType": "address",
				"name": "from",
				"type": "address"
			},
			{
				"internalType": "address",
				"name": "to",
				"type": "address"
			},
			{
				"internalType": "uint256",
				"name": "tokenId",
				"type": "uint256"
			}
		],
		"name": "safeTransferFrom",
		"outputs": [],
		"stateMutability": "nonpayable",
		"type": "function"
	},
	{
		"inputs": [
			{
				"internalType": "address",
				"name": "from",
				"type": "address"
			},
			{
				"internalType": "address",
				"name": "to",
				"type": "address"
			},
			{
				"internalType": "uint256",
				"name": "tokenId",
				"type": "uint256"
			},
			{
				"internalType": "bytes",
				"name": "data",
				"type": "bytes"
			}
		],
		"name": "safeTransferFrom",
		"outputs": [],
		"stateMutability": "nonpayable",
		"type": "function"
	},
	{
		"inputs": [
			{
				"internalType": "address",
				"name": "operator",
				"type": "address"
			},
			{
				"internalType": "bool",
				"name": "_approved",
				"type": "bool"
			}
		],
		"name": "setApprovalForAll",
		"outputs": [],
		"stateMutability": "nonpayable",
		"type": "function"
	},
	{
		"inputs": [
			{
				"internalType": "bytes4",
				"name": "interfaceId",
				"type": "bytes4"
			}
		],
		"name": "supportsInterface",
		"outputs": [
			{
				"internalType": "bool",
				"name": "",
				"type": "bool"
			}
		],
		"stateMutability": "view",
		"type": "function"
	},
	{
		"inputs": [],
		"name": "symbol",
		"outputs": [
			{
				"internalType": "string",
				"name": "",
				"type": "string"
			}
		],
		"stateMutability": "view",
		"type": "function"
	},
	{
		"inputs": [
			{
				"internalType": "uint256",
				"name": "index",
				"type": "uint256"
			}
		],
		"name": "tokenByIndex",
		"outputs": [
			{
				"internalType": "uint256",
				"name": "",
				"type": "uint256"
			}
		],
		"stateMutability": "view",
		"type": "function"
	},
	{
		"inputs": [
			{
				"internalType": "address",
				"name": "owner",
				"type": "address"
			},
			{
				"internalType": "uint256",
				"name": "index",
				"type": "uint256"
			}
		],
		"name": "tokenOfOwnerByIndex",
		"outputs": [
			{
				"internalType": "uint256",
				"name": "tokenId",
				"type": "uint256"
			}
		],
		"stateMutability": "view",
		"type": "function"
	},
	{
		"inputs": [
			{
				"internalType": "uint256",
				"name": "tokenId",
				"type": "uint256"
			}
		],
		"name": "tokenURI",
		"outputs": [
			{
				"internalType": "string",
				"name": "",
				"type": "string"
			}
		],
		"stateMutability": "view",
		"type": "function"
	},
	{
		"inputs": [],
		"name": "totalSupply",
		"outputs": [
			{
				"internalType": "uint256",
				"name": "",
				"type": "uint256"
			}
		],
		"stateMutability": "view",
		"type": "function"
	},
	{
		"inputs": [
			{
				"internalType": "address",
				"name": "from",
				"type": "address"
			},
			{
				"internalType": "address",
				"name": "to",
				"type": "address"
			},
			{
				"internalType": "uint256",
				"name": "tokenId",
				"type": "uint256"
			}
		],
		"name": "transferFrom",
		"outputs": [],
		"stateMutability": "nonpayable",
		"type": "function"
	}
]
```
{% endcode %}

Now that we have our ABI in our project let's add a Rust build script.

{% hint style="info" %}
**Rust Build Script**

Just before a package is built, Cargo will compile a build script into an executable (if it has not already been built). It will then run the script, which may perform any number of tasks.&#x20;

Placing a file named `build.rs` in the root of a package will cause Cargo to compile that script and execute it just before building the package
{% endhint %}

We will create a `build.rs` file in the root of our Substreams directory

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

We will run the build script by building the project&#x20;

```bash
cargo build --target wasm32-unknown-unknown --release
```

You should now have a generated ABI folder `src/abi` we will create a `mod.rs` file in that folder to export the generated Rust code

{% code title="src/abi/mod.rs" %}
```rust
pub mod erc721;
```
{% endcode %}

Now that we have our ABI & Protobuf Rust code generated lets write our handler code in `src/lib.rs`

{% code title="src/lib.rs" %}
```rust
mod abi;
mod pb;
use hex_literal::hex;
use pb::erc721;
use substreams::{log, store, Hex};
use substreams_ethereum::{pb::eth::v1 as eth, EMPTY_ADDRESS};

// Bored Ape Club Contract
const TRACKED_CONTRACT: [u8; 20] = hex!("bc4ca0eda7647a8ab7c2061c2e118a18a936f13d");

substreams_ethereum::init!();

/// Extracts transfers events from the contract
#[substreams::handlers::map]
fn block_to_transfers(blk: eth::Block) -> Result<erc721::Transfers, substreams::errors::Error> {
    let mut transfers: Vec<erc721::Transfer> = vec![];
    for trx in blk.transaction_traces {
        transfers.extend(trx.receipt.unwrap().logs.iter().filter_map(|log| {
            if log.address != TRACKED_CONTRACT {
                return None;
            }

            log::debug!("NFT Contract {} invoked", Hex(&TRACKED_CONTRACT));

            if !abi::erc721::events::Transfer::match_log(log) {
                return None;
            }

            let transfer = abi::erc721::events::Transfer::must_decode(log);

            Some(erc721::Transfer {
                trx_hash: trx.hash.clone(),
                from: transfer.from,
                to: transfer.to,
                token_id: transfer.token_id.low_u64(),
                ordinal: log.block_index as u64,
            })
        }));
    }

    Ok(erc721::Transfers { transfers })
}

// Store the total balance of NFT tokens by address for the specific TRACKED_CONTRACT by holder
/// Store the total balance of NFT tokens for the specific TRACKED_CONTRACT by holder
#[substreams::handlers::store]
fn nft_state(transfers: erc721::Transfers, s: store::StoreAddInt64) {
    log::info!("NFT state builder");
    for transfer in transfers.transfers {
        if transfer.from != EMPTY_ADDRESS {
            log::info!("Found a transfer out");

            s.add(transfer.ordinal, generate_key(&transfer.from), -1);
        }

        if transfer.to != EMPTY_ADDRESS {
            log::info!("Found a transfer in");

            s.add(transfer.ordinal, generate_key(&transfer.to), 1);
        }
    }
}

fn generate_key(holder: &Vec<u8>) -> String {
    return format!("total:{}:{}", Hex(holder), Hex(TRACKED_CONTRACT));
}


```
{% endcode %}

**Let's break down**

We setup our imports

```rust
mod abi;
mod pb;
use hex_literal::hex;
use pb::erc721;
use substreams::{log, store, Hex};
use substreams_ethereum::{pb::eth::v1 as eth, EMPTY_ADDRESS};
...
```

We store as a `constant` the contract we are tracking, and initiate our Ethereum Substream

```rust
...

// Bored Ape Club Contract
const TRACKED_CONTRACT: [u8; 20] = hex!("bc4ca0eda7647a8ab7c2061c2e118a18a936f13d");

substreams_ethereum::init!();

...
```

We now define our first `map` module. As a reminder here is the module definition in the Manfiest&#x20;

```yaml
  - name: block_to_transfers
    kind: map
    startBlock: 12287507
    code:
      type: wasm/rust-v1
      file: ./target/wasm32-unknown-unknown/release/substreams_nft_holders.wasm
      entrypoint: block_to_transfers
    inputs:
      - source: sf.ethereum.type.v1.Block
    output:
      type: proto:eth.erc721.v1.Transfers
```

First notice the `entrypoint: block_to_transfers` this should correspond to our handler function name.&#x20;

Secondly we have defined 1 input and 1 output,. The input has a type of `sf.ethereum.type.v1.Block`, this is a standard Ethereum block that is provided by the `substreams-ethereum` crate. The output has a type of `proto:eth.erc721.v1.Transfers` this is our custom Protobuf definition and is provided by the generated Rust code we did in the prior steps. This yields the following function signature

```rust
...

/// Extracts transfers events from the contract
#[substreams::handlers::map]
fn block_to_transfers(blk: eth::Block) -> Result<erc721::Transfers, substreams::errors::Error> {
    ...
}

...
```

{% hint style="info" %}
**Rust Macros**

Notice the `#[substreams::handlers::map]` above the function, this is a [rust macro](https://doc.rust-lang.org/book/ch19-06-macros.html) that is provided by the `substreams` crate. This macro decorates our handler function as a `map`. There is also a macro used to decorate handler of kind `store`:&#x20;

`#[substreams::handlers::store]`&#x20;

To learn how these macros work you can view this in the [advanced section](../reference-and-specs/advanced/rust-macros.md)
{% endhint %}

{% hint style="info" %}
**Handler Function Signature**

The Rust function signature of a handler is derived based on the module kind, inputs and output define in your manifest. There are few combination that can work. You get an in-depth overview of this in the [reference section](../reference-and-specs/rust-handler-signature.md)
{% endhint %}

The goal of the `map` we are building is to extract `ERC721` Transfers from a given block. We can achieve this by finding all the `Transfer` events that are emitted by the contract we are tracking. Once we find such an event we will decode it and create a `Transfer` object

```rust
...

/// Extracts transfers events from the contract
#[substreams::handlers::map]
fn block_to_transfers(blk: eth::Block) -> Result<erc721::Transfers, substreams::errors::Error> {
    // variable to store the transfers we find
    let mut transfers: Vec<erc721::Transfer> = vec![];
    // loop through the block's transaction
    for trx in blk.transaction_traces {
        // iterate over the transaction logs
        transfers.extend(trx.receipt.unwrap().logs.iter().filter_map(|log| {
            // verifying if the log are emitted by the contract we are tracking
            if log.address != TRACKED_CONTRACT {
                return None;
            }

            log::debug!("NFT Contract {} invoked", Hex(&TRACKED_CONTRACT));
            // check if the 
            if !abi::erc721::events::Transfer::match_log(log) {
                return None;
            }

            let transfer = abi::erc721::events::Transfer::must_decode(log);

            Some(erc721::Transfer {
                trx_hash: trx.hash.clone(),
                from: transfer.from,
                to: transfer.to,
                token_id: transfer.token_id.low_u64(),
                ordinal: log.block_index as u64,
            })
        }));
    }

    Ok(erc721::Transfers { transfers })
}

```

