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
- **Firehose Solana** (port 9000) - Firehose integration providing gRPC streaming
- **Docker network** - Connecting all services

```
┌─────────────────┐    ┌──────────────────┐    ┌─────────────────┐
│   Your App      │    │    Substreams    │    │     Anchor      │
│                 │    │                  │    │                 │
│ ┌─────────────┐ │    │ ┌──────────────┐ │    │ ┌─────────────┐ │
│ │ Substreams  │◄┼────┼─┤   Firehose   │ │    │ │   Deploy    │ │
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
version: '3.8'

services:
  solana-validator:
    image: solana/solana:v1.18.26
    container_name: solana-dev-validator
    command: |
      solana-test-validator
      --rpc-port 8899
      --rpc-bind-address 0.0.0.0
      --faucet-port 8900
      --faucet-sol 1000000
      --enable-rpc-transaction-history
      --enable-extended-tx-metadata-storage
      --log
    ports:
      - "8899:8899"   # RPC
      - "8900:8900"   # Faucet
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

  firehose-solana:
    image: ghcr.io/streamingfast/firehose-solana:v1.1.0
    container_name: firehose-solana
    command: |
      firehose-solana start
      --config-file=
      --common-first-streamable-block=0
      --firehose-grpc-listen-addr=:9000
      --reader-node-path=firesolana
      --reader-node-arguments="fetch rpc http://solana-validator:8899"
    ports:
      - "9000:9000"   # Firehose gRPC
    depends_on:
      solana-validator:
        condition: service_healthy
    healthcheck:
      test: ["CMD", "grpc_health_probe", "-addr=localhost:9000"]
      interval: 10s
      timeout: 5s
      retries: 5
      start_period: 60s
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
docker-compose up -d
```

Wait for the services to start (about 60-90 seconds).

## Validation Commands

### 1. Check Docker Services

Verify all containers are running and healthy:

```bash
docker-compose ps
```

Expected output:
```
NAME                   COMMAND                  SERVICE           STATUS              PORTS
solana-dev-validator   "solana-test-validat…"   solana-validator  Up (healthy)        0.0.0.0:8899->8899/tcp, 0.0.0.0:8900->8900/tcp
firehose-solana        "firehose-solana sta…"   firehose-solana   Up (healthy)        0.0.0.0:9000->9000/tcp
```

### 2. Test RPC Connectivity

Test the Solana JSON-RPC endpoint:

```bash
curl -X POST -H "Content-Type: application/json" \
  --data '{"jsonrpc":"2.0","id":1,"method":"getHealth"}' \
  http://localhost:8899
```

Expected response:
```json
{"jsonrpc":"2.0","result":"ok","id":1}
```

### 3. Test Firehose Connectivity

Test Firehose gRPC connectivity:

```bash
docker run --rm --network=host ghcr.io/streamingfast/firehose-solana:latest \
  firecore tools firehose-client localhost:9000 --plaintext -o text -- -1
```

Or using substreams CLI:

```bash
docker run --rm --network=host ghcr.io/streamingfast/substreams:latest \
  substreams run -e localhost:9000 --plaintext common@v0.1.0 -s -1
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
mkdir anchor-counter
cd anchor-counter
anchor init counter --no-git
cd counter
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

### 1. Initialize Substreams Project

```bash
cd .. # Back to substreams-solana-local
mkdir substreams
cd substreams
```

### 2. Create Protobuf Schema

Create `proto/counter.proto`:

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

### Docker Issues

**Problem:** Container fails to start
```bash
# Check logs
docker-compose logs solana-validator
docker-compose logs firehose-solana

# Restart services
docker-compose down
docker-compose up -d
```

**Problem:** Port conflicts
```bash
# Check what's using the ports
lsof -i :8899
lsof -i :9000

# Kill conflicting processes or change ports in docker-compose.yml
```

### Solana Validator Issues

**Problem:** Connection refused to localhost:8899
- Ensure Docker container is running and healthy
- Check firewall settings
- Verify port mapping in docker-compose.yml

**Problem:** "Health check failed"
- Wait for validator to fully initialize (can take 60+ seconds)
- Check validator logs: `docker-compose logs solana-validator`

### Anchor/CLI Issues

**Problem:** "insufficient funds for transaction"
- Run `solana airdrop 10` to get more SOL
- Verify balance: `solana balance`

**Problem:** "Program not found" during deployment
- Ensure Solana CLI is configured correctly: `solana config get`
- Verify validator is running and accessible

**Problem:** Anchor build fails
- Ensure Rust and Anchor versions are correct
- Check `Anchor.toml` configuration

### Firehose Connectivity Issues

**Problem:** gRPC connection failed
- Verify Firehose is listening on port 9000
- Check Docker network connectivity
- Ensure `--plaintext` flag is used for local development
- Wait for both services to be healthy

### Substreams Build Issues

**Problem:** `wasm32-unknown-unknown` target not found
```bash
rustup target add wasm32-unknown-unknown
```

**Problem:** Protobuf generation fails
- Ensure `substreams protogen` excludes system paths
- Check proto file syntax

**Problem:** No events in Substreams output
- Verify program ID in substreams.yaml matches deployed program
- Check that transactions actually generated events
- Ensure slot range includes deployment slot
- Verify Anchor event discriminators are correct

### Mac-Specific Networking

**Problem:** Docker networking issues on macOS
- Use `host.docker.internal` instead of `localhost` in some contexts
- Ensure Docker Desktop networking is properly configured

{% hint style="warning" %}
Local validator provides unlimited SOL - production networks require real SOL for transactions.
{% endhint %}

## Next Steps

Now that you have a working local environment:

1. **Extend the Program** - Add more instructions and events to explore different patterns
2. **Advanced Substreams** - Implement stores, multiple modules, and complex data transformations  
3. **Testing Scenarios** - Create reproducible test cases with specific program states
4. **Integration** - Connect your Substreams to sinks like databases or message queues

## Additional Resources

- [Anchor Documentation](https://www.anchor-lang.com/)
- [Solana Cookbook](https://solanacookbook.com/)
- [Substreams Solana Reference](https://github.com/streamingfast/substreams-solana)
- [Solana Test Validator](https://docs.solana.com/developing/test-validator)
