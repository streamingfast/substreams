# Solana Local Development with Anchor

This guide walks you through setting up a complete local Solana development environment for Substreams development using Anchor. You'll deploy a sample Counter program, generate transactions, and stream the events using Substreams.

**Estimated time:** 20-25 minutes

## What You'll Build

- Local Solana validator (test mode)
- Firehose integration for block streaming
- Counter Anchor program with events
- Substreams module to extract program events
- Complete Docker Compose orchestration

## Prerequisites

Ensure you have the following installed:

- **Docker 20.10+** with Docker Compose v2.0+
- **Node.js 18+** with npm or yarn
- **Anchor CLI** 0.30.1 - [Installation guide](https://www.anchor-lang.com/docs/installation)
- **Solana CLI** 1.18.26 - [Installation guide](https://docs.solana.com/cli/install-solana-cli-tools)
- **Substreams CLI** v1.7.0+ ([installation guide](../../../cli/installing-the-cli.md))
- **Rust** with `wasm32-unknown-unknown` target
- **curl** for testing endpoints

## Architecture Overview

The local environment consists of:

- **Solana Validator** (port 8899/8900) - Test validator with unlimited SOL
- **Substreams** (port 9000) - Substreams Tier1 service providing gRPC streaming
- **Docker network** - Connecting all services

```
┌─────────────────┐    ┌──────────────────┐    ┌─────────────────┐
│   Your App      │    │    Substreams    │    │     Anchor      │
│                 │    │                  │    │                 │
│ ┌─────────────┐ │    │ ┌──────────────┐ │    │ ┌─────────────┐ │
│ │ Substreams  │◄┼────┼─┤  Substreams  │ │    │ │   Deploy    │ │
│ │    CLI      │ │    │ │   (port      │ │    │ │  Programs   │ │
│ └─────────────┘ │    │ │    9000)     │ │    │ └─────────────┘ │
└─────────────────┘    │ └──────────────┘ │    └─────────────────┘
                       │         │        │
                       │ ┌──────────────┐ │
                       │ │   Solana     │ │
                       │ │ Validator    │ │
                       │ │  (ports      │ │
                       │ │ 8899/8900)   │ │
                       │ └──────────────┘ │
                       └──────────────────┘
```

## Setup Instructions

### 1. Create Project Directory

```bash
mkdir substreams-solana-local
cd substreams-solana-local
```

### 2. Create Docker Compose Configuration

Create a `docker-compose.yml` file:

```yaml
services:
  solana-node:
    image: ghcr.io/beeman/solana-test-validator:2.2.15
    container_name: solana-dev-validator
    entrypoint: ["solana-test-validator"]
    command:
      - --rpc-port=8899
      - --rpc-bind-address=0.0.0.0
      - --faucet-port=8900
      - --faucet-sol=1000000
      - --enable-rpc-transaction-history
      - --enable-extended-tx-metadata-storage
      - --log
    ports:
      - "8899:8899"
      - "8900:8900"
    healthcheck:
      test: ["CMD", "curl", "-f", "http://localhost:8899", "-X", "POST", "-H", "Content-Type: application/json", "-d", '{"jsonrpc":"2.0","id":1,"method":"getHealth"}']
      interval: 10s
      timeout: 5s
      retries: 5
      start_period: 30s
    volumes:
      - solana_data:/data
    networks:
      - solana_network

  bigeagle-firehose:
    image: ghcr.io/streamingfast/firehose-solana:v1.1.0
    container_name: firehose-solana
    entrypoint: ["/app/firecore"]
    command:
      - start
      - reader-node,merger,relayer,firehose,substreams-tier1,substreams-tier2
      - --config-file=
      - --log-format=text
      - --log-to-file=false
      - --common-first-streamable-block=0
      - --firehose-grpc-listen-addr=:8089
      - --substreams-tier1-grpc-listen-addr=:9000
      - --substreams-tier1-block-type=sf.solana.type.v1.Block
      - --advertise-block-id-encoding=base58
      - --reader-node-path=/app/firesol
      - --reader-node-arguments=fetch rpc http://solana-node:8899 --state-dir=/data/reader-state
    ports:
      - "8089:8089"
      - "9000:9000"
    depends_on:
      solana-node:
        condition: service_healthy
    healthcheck:
      test: ["CMD", "grpc_health_probe", "-addr=localhost:9000"]
      interval: 10s
      timeout: 5s
      retries: 5
      start_period: 60s
    volumes:
      - solana_data:/data
    networks:
      - solana_network

volumes:
  solana_data:

networks:
  solana_network:
    driver: bridge
```

### 3. Start the Environment

```bash
docker compose up -d
```

{% hint style="warning" %}
To restart everything from scratch, use `docker compose down --volumes` to remove all data and start fresh.
{% endhint %}

## Validation Commands

### 1. Check Docker Services

Verify all containers are running and healthy:

```bash
docker compose ps
```

Expected output:
```
NAME                   COMMAND                  SERVICE           STATUS              PORTS
solana-dev-validator   "solana-test-validat…"   solana-node       Up (healthy)        0.0.0.0:8899->8899/tcp, 0.0.0.0:8900->8900/tcp
firehose-solana        "/app/firecore start…"   bigeagle-firehose Up (healthy)        0.0.0.0:8089->8089/tcp, 0.0.0.0:9000->9000/tcp
firehose-solana        "firehose-solana sta…"   firehose-solana   Up (healthy)        0.0.0.0:9000->9000/tcp
```

### 2. Test Substreams Connectivity

Test Substreams Tier1 gRPC connectivity:

```bash
substreams run -e localhost:9000 --plaintext common@v0.1.0 -o clock -s -1
```

Expected output:
```text
Writing clock information only (no data)
----------- BLOCK #24 (6c3dbc20ae11cb856bed9789f7845359e98de71b830f0d9599d0061ed4e962d2) age=1.959195s ---------------
...
```

{% hint style="success" %}
If all validation commands succeed, your environment is ready!
{% endhint %}

## Deploy Counter Program with Anchor

### 1. Install Required Tools

Install Anchor CLI if not already installed:

```bash
cargo install avm
avm install 0.30.1
avm use 0.30.1
```

Verify installation:
```bash
anchor --version  # Should show 0.30.1
solana --version  # Should show 1.18.26
```

### 2. Configure Solana CLI

```bash
# Set cluster to local
solana config set --url http://localhost:8899

# Create a keypair (or use existing)
solana-keygen new --outfile ~/.config/solana/id.json

# Airdrop SOL for deployment
solana airdrop 10
```

### 3. Initialize Anchor Project

```bash
anchor init counter --no-git
```

### 4. Configure Anchor

Update `Anchor.toml`:

```toml
[features]
seeds = false
skip-lint = false

[programs.localnet]
counter = "Fg6PaFpoGXkYsidMpWTK6W2BeZ7FEfcYkg476zPFsLnS"

[registry]
url = "https://api.apr.dev"

[provider]
cluster = "Localnet"
wallet = "~/.config/solana/id.json"

[scripts]
test = "yarn run ts-mocha -p ./tsconfig.json -t 1000000 tests/**/*.ts"

[[test.genesis]]
address = "Fg6PaFpoGXkYsidMpWTK6W2BeZ7FEfcYkg476zPFsLnS"
program = "target/deploy/counter.so"
```

### 5. Create Counter Program

Update `programs/counter/src/lib.rs`:

```rust
use anchor_lang::prelude::*;

declare_id!("Fg6PaFpoGXkYsidMpWTK6W2BeZ7FEfcYkg476zPFsLnS");

#[program]
pub mod counter {
    use super::*;

    pub fn initialize(ctx: Context<Initialize>, initial_count: u64) -> Result<()> {
        let counter = &mut ctx.accounts.counter;
        counter.count = initial_count;
        counter.authority = ctx.accounts.authority.key();
        
        emit!(CounterInitialized {
            counter: counter.key(),
            initial_count,
            authority: ctx.accounts.authority.key(),
        });
        
        Ok(())
    }

    pub fn increment(ctx: Context<Update>) -> Result<()> {
        let counter = &mut ctx.accounts.counter;
        counter.count += 1;
        
        emit!(CounterIncremented {
            counter: counter.key(),
            new_count: counter.count,
            caller: ctx.accounts.authority.key(),
        });
        
        Ok(())
    }

    pub fn decrement(ctx: Context<Update>) -> Result<()> {
        let counter = &mut ctx.accounts.counter;
        require!(counter.count > 0, CounterError::CannotDecrementBelowZero);
        counter.count -= 1;
        
        emit!(CounterDecremented {
            counter: counter.key(),
            new_count: counter.count,
            caller: ctx.accounts.authority.key(),
        });
        
        Ok(())
    }
}

#[derive(Accounts)]
pub struct Initialize<'info> {
    #[account(
        init,
        payer = authority,
        space = 8 + Counter::INIT_SPACE
    )]
    pub counter: Account<'info, Counter>,
    #[account(mut)]
    pub authority: Signer<'info>,
    pub system_program: Program<'info, System>,
}

#[derive(Accounts)]
pub struct Update<'info> {
    #[account(
        mut,
        has_one = authority @ CounterError::Unauthorized
    )]
    pub counter: Account<'info, Counter>,
    pub authority: Signer<'info>,
}

#[account]
#[derive(InitSpace)]
pub struct Counter {
    pub count: u64,
    pub authority: Pubkey,
}

#[event]
pub struct CounterInitialized {
    pub counter: Pubkey,
    pub initial_count: u64,
    pub authority: Pubkey,
}

#[event]
pub struct CounterIncremented {
    pub counter: Pubkey,
    pub new_count: u64,
    pub caller: Pubkey,
}

#[event]
pub struct CounterDecremented {
    pub counter: Pubkey,
    pub new_count: u64,
    pub caller: Pubkey,
}

#[error_code]
pub enum CounterError {
    #[msg("Cannot decrement counter below zero")]
    CannotDecrementBelowZero,
    #[msg("Unauthorized access")]
    Unauthorized,
}
```

### 6. Create Deployment Test

Update `tests/counter.ts`:

```typescript
import * as anchor from "@coral-xyz/anchor";
import { Program } from "@coral-xyz/anchor";
import { Counter } from "../target/types/counter";
import { expect } from "chai";

describe("counter", () => {
  const provider = anchor.AnchorProvider.env();
  anchor.setProvider(provider);

  const program = anchor.workspace.Counter as Program<Counter>;
  const counterKeypair = anchor.web3.Keypair.generate();

  it("Initializes the counter", async () => {
    console.log("Initializing counter...");
    
    const tx = await program.methods
      .initialize(new anchor.BN(0))
      .accounts({
        counter: counterKeypair.publicKey,
        authority: provider.wallet.publicKey,
        systemProgram: anchor.web3.SystemProgram.programId,
      })
      .signers([counterKeypair])
      .rpc();

    console.log("Initialize transaction signature:", tx);

    const counterAccount = await program.account.counter.fetch(counterKeypair.publicKey);
    expect(counterAccount.count.toNumber()).to.equal(0);
    
    console.log(`🎉 SAVE THIS PROGRAM ID: ${program.programId.toString()}`);
    console.log(`🎉 SAVE THIS COUNTER ADDRESS: ${counterKeypair.publicKey.toString()}`);
  });

  it("Increments the counter", async () => {
    console.log("Incrementing counter...");
    
    const tx1 = await program.methods
      .increment()
      .accounts({
        counter: counterKeypair.publicKey,
        authority: provider.wallet.publicKey,
      })
      .rpc();

    console.log("Increment transaction 1 signature:", tx1);

    const tx2 = await program.methods
      .increment()
      .accounts({
        counter: counterKeypair.publicKey,
        authority: provider.wallet.publicKey,
      })
      .rpc();

    console.log("Increment transaction 2 signature:", tx2);

    const counterAccount = await program.account.counter.fetch(counterKeypair.publicKey);
    expect(counterAccount.count.toNumber()).to.equal(2);
    
    console.log("Final count:", counterAccount.count.toNumber());
  });
});
```

### 7. Build and Deploy

```bash
# Build the program
anchor build

# Deploy to local validator
anchor deploy

# Run tests to generate transactions
anchor test --skip-local-validator
```

**SAVE THE PROGRAM ID** from the deployment output!

### 8. Verify Deployment

```bash
# Check program account
solana program show <PROGRAM_ID>

# Check if program is deployed
solana account <PROGRAM_ID>
```

{% hint style="info" %}
Save your deployed program ID - you'll need it for the Substreams configuration.
{% endhint %}

{% hint style="note" %}
Solana uses slots (400ms) - multiple slots can be in one block. The test validator provides unlimited SOL via airdrop.
{% endhint %}

## Create Substreams Module

### 1. Extract IDL File

First, extract the Anchor IDL from your deployed program:

```bash
anchor idl fetch <PROGRAM_ID> --filepath counter.json
```

### 2. Initialize Substreams Project

Use the interactive `substreams init` command to bootstrap your project:

```bash
substreams init
```

Follow the interactive prompts:
- **Chosen protocol**: `Solana`
- **Chosen generator**: `solana-program-instructions`
- **Please enter the project name**: `counter`
- **Please select the chain**: `Solana Mainnet` (or your target chain)
- **Please enter the program address**: `<PROGRAM_ID>` (from your deployment)
- **How do you want to provide the JSON IDL?**: `JSON in a local file`
- **Input the full path of the JSON IDL**: `counter.json`
- **Please enter the program initial block number**: `0`
- **Choose a short name for the program**: `counter`
- **What do you want to track for this program?**: `Instructions`
- **Add another program?**: `No`
- **In which directory do you want to download the project?**: `./substreams`
- **How would you like to consume the Substreams?**: `To Postgres` (or choose any other option)

### 3. Build and Test Substreams

```bash
cd substreams
substreams build
substreams run -e localhost:9000 --plaintext counter-v0.1.0.spkg -s <DEPLOYMENT_BLOCK> -t +10
```

{% hint style="note" %}
Look for the deployment block in your transaction output - it's the block that will contain actual program data. Use `-s <DEPLOYMENT_BLOCK> -t +10` to scan a specific range of blocks.
{% endhint %}

## Troubleshooting

```protobuf
syntax = "proto3";

package counter.v1;

message CounterEvents {
  repeated CounterEvent events = 1;
}

message CounterEvent {
  uint64 slot = 1;
  string tx_id = 2;
  string event_type = 3; // "Initialized", "Incremented", "Decremented"
  string program_id = 4;
  string counter_address = 5;
  uint64 count = 6;
  string authority = 7;
  uint32 instruction_index = 8;
}
```

### 3. Create Substreams Manifest

Create `substreams.yaml`:

```yaml
specVersion: v0.1.0
package:
  name: counter_events
  version: v0.1.0

imports:
  solana: https://github.com/streamingfast/substreams-solana/releases/download/v0.8.0/substreams-solana-v0.8.0.spkg

protobuf:
  files:
    - counter.proto
  importPaths:
    - ./proto

binaries:
  default:
    type: wasm/rust-v1
    file: ./target/wasm32-unknown-unknown/release/substreams.wasm

modules:
  - name: map_counter_events
    kind: map
    initialBlock: 0
    inputs:
      - params: string
      - source: sf.solana.type.v1.Block
    output:
      type: proto:counter.v1.CounterEvents

params:
  map_counter_events: "program=11111111111111111111111111111111" # Replace with your program ID

network: local
```

### 4. Create Cargo Configuration

Create `Cargo.toml`:

```toml
[package]
name = "substreams-counter"
version = "0.1.0"
edition = "2021"

[lib]
crate-type = ["cdylib"]

[dependencies]
substreams = "0.6"
substreams-solana = "0.8"
prost = "0.12"
prost-types = "0.12"
hex = "0.4"
base58 = "0.2"
```

### 5. Create Rust Handler

Create `src/lib.rs`:

```rust
use substreams::prelude::*;
use substreams_solana::pb::sf::solana::type::v1 as solana;

mod pb;
use pb::counter::v1::{CounterEvent, CounterEvents};

// Anchor event discriminators (first 8 bytes of event data)
const COUNTER_INITIALIZED_DISCRIMINATOR: [u8; 8] = [119, 26, 70, 90, 111, 31, 112, 54];
const COUNTER_INCREMENTED_DISCRIMINATOR: [u8; 8] = [11, 65, 208, 137, 112, 17, 87, 123];
const COUNTER_DECREMENTED_DISCRIMINATOR: [u8; 8] = [106, 153, 111, 72, 151, 88, 49, 11];

#[substreams::handlers::map]
fn map_counter_events(params: String, block: solana::Block) -> Result<CounterEvents, substreams::errors::Error> {
    let program_id = extract_program_id(&params)?;
    
    let mut events = Vec::new();
    
    for confirmed_transaction in block.transactions {
        if let Some(transaction) = confirmed_transaction.transaction {
            if let Some(meta) = confirmed_transaction.meta {
                // Check if transaction was successful
                if meta.err.is_some() {
                    continue;
                }
                
                // Process log messages for Anchor events
                for (log_index, log_message) in meta.log_messages.iter().enumerate() {
                    if let Some(event) = parse_anchor_event(
                        log_message,
                        &program_id,
                        block.slot,
                        &base58::encode(&transaction.signatures[0]),
                        log_index as u32,
                    ) {
                        events.push(event);
                    }
                }
            }
        }
    }
    
    Ok(CounterEvents { events })
}

fn extract_program_id(params: &str) -> Result<String, substreams::errors::Error> {
    for param in params.split('&') {
        if let Some(program) = param.strip_prefix("program=") {
            return Ok(program.to_string());
        }
    }
    Err(substreams::errors::Error::Unexpected("Program ID parameter not found".to_string()))
}

fn parse_anchor_event(
    log_message: &str,
    program_id: &str,
    slot: u64,
    tx_id: &str,
    instruction_index: u32,
) -> Option<CounterEvent> {
    // Anchor events are logged as "Program data: <base64_data>"
    if !log_message.starts_with("Program data: ") {
        return None;
    }
    
    let data_part = &log_message[14..]; // Skip "Program data: "
    let event_data = match base64::decode(data_part) {
        Ok(data) => data,
        Err(_) => return None,
    };
    
    if event_data.len() < 8 {
        return None;
    }
    
    let discriminator = &event_data[0..8];
    
    match discriminator {
        &COUNTER_INITIALIZED_DISCRIMINATOR => {
            if event_data.len() >= 72 { // 8 + 32 + 8 + 32 = 80 bytes minimum
                let counter_address = base58::encode(&event_data[8..40]);
                let initial_count = u64::from_le_bytes([
                    event_data[40], event_data[41], event_data[42], event_data[43],
                    event_data[44], event_data[45], event_data[46], event_data[47],
                ]);
                let authority = base58::encode(&event_data[48..80]);
                
                Some(CounterEvent {
                    slot,
                    tx_id: tx_id.to_string(),
                    event_type: "Initialized".to_string(),
                    program_id: program_id.to_string(),
                    counter_address,
                    count: initial_count,
                    authority,
                    instruction_index,
                })
            } else {
                None
            }
        }
        &COUNTER_INCREMENTED_DISCRIMINATOR => {
            if event_data.len() >= 72 {
                let counter_address = base58::encode(&event_data[8..40]);
                let new_count = u64::from_le_bytes([
                    event_data[40], event_data[41], event_data[42], event_data[43],
                    event_data[44], event_data[45], event_data[46], event_data[47],
                ]);
                let caller = base58::encode(&event_data[48..80]);
                
                Some(CounterEvent {
                    slot,
                    tx_id: tx_id.to_string(),
                    event_type: "Incremented".to_string(),
                    program_id: program_id.to_string(),
                    counter_address,
                    count: new_count,
                    authority: caller,
                    instruction_index,
                })
            } else {
                None
            }
        }
        &COUNTER_DECREMENTED_DISCRIMINATOR => {
            if event_data.len() >= 72 {
                let counter_address = base58::encode(&event_data[8..40]);
                let new_count = u64::from_le_bytes([
                    event_data[40], event_data[41], event_data[42], event_data[43],
                    event_data[44], event_data[45], event_data[46], event_data[47],
                ]);
                let caller = base58::encode(&event_data[48..80]);
                
                Some(CounterEvent {
                    slot,
                    tx_id: tx_id.to_string(),
                    event_type: "Decremented".to_string(),
                    program_id: program_id.to_string(),
                    counter_address,
                    count: new_count,
                    authority: caller,
                    instruction_index,
                })
            } else {
                None
            }
        }
        _ => None,
    }
}
```

### 6. Generate Protobuf Code

```bash
substreams protogen --exclude-paths="google,sf/substreams"
```

### 7. Build Substreams Module

```bash
cargo build --target wasm32-unknown-unknown --release
```

### 8. Update Program ID

**IMPORTANT:** Update the program ID in `substreams.yaml`:

```yaml
params:
  map_counter_events: "program=YOUR_PROGRAM_ID_HERE" # Replace with actual program ID
```

### 9. Test Substreams

```bash
substreams gui -e localhost:9000 --plaintext map_counter_events -s 0
```

You should see the Counter events from your program deployment and test transactions!

## Troubleshooting

For common issues with Docker Compose, RPC connectivity, and Substreams, see the [Local Development Troubleshooting](../../generic/local-development/troubleshooting.md) guide.

## Next Steps

Now that you have a working local Solana development environment:

1. **Explore Other Local Development Guides:**
   - [Ethereum with HardHat](../../evm/local-development/hardhat.md) - EVM development with HardHat 3 Beta
   - [Ethereum with Foundry](../../evm/local-development/foundry.md) - EVM development with Foundry toolkit

2. **Advanced Substreams Development:**
   - [Composing Substreams](../../../composing-substreams/composing-substreams.md) - Build complex data pipelines
   - [Foundational Modules](../../../composing-substreams/foundational-modules.md) - Reusable Substreams components

3. **Production Deployment:**
   - [Consuming Substreams](../../../sinks/sinks.md) - Connect to databases and services
   - [Substreams:SQL](../../../sinks/sql/sql.md) - Stream data to PostgreSQL/ClickHouse
   - [Publishing Packages](../../../publish-package.md) - Share your Substreams modules

4. **Solana-Specific Resources:**
   - [SPL Token Tracker](../token-tracker/token-tracker.md) - Track Solana token transfers
   - [DEX Trades](../top-ledger/dex-trades.md) - Monitor decentralized exchange activity

## Additional Resources

- [Anchor Documentation](https://www.anchor-lang.com/) - Solana development framework
- [Solana Cookbook](https://solanacookbook.com/) - Solana development recipes
- [Substreams Solana Reference](https://github.com/streamingfast/substreams-solana) - Solana-specific Substreams tools
- [Substreams CLI Reference](../../../../references/cli/command-line-interface.md) - Complete CLI documentation
