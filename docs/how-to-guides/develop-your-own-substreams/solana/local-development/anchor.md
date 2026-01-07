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
- **Anchor CLI** (latest) - [Installation guide](https://www.anchor-lang.com/docs/installation)
- **Solana CLI** 2.x+ - [Installation guide](https://docs.solana.com/cli/install-solana-cli-tools)
- **Substreams CLI** v1.7.0+ ([installation guide](../../../cli/installing-the-cli.md))
- **Rust** with `wasm32-unknown-unknown` target
- **curl** for testing endpoints

## Architecture Overview

The local environment consists of:

- **Solana Validator** (port 8899/8900) - Test validator with unlimited SOL
- **Substreams** (port 9000) - Substreams Tier1 service providing gRPC streaming
- **Docker network** - Connecting all services

```
┌─────────────────┐    ┌─────────────────────┐    ┌─────────────────┐
│   Your App      │    │     Substreams      │    │      Anchor     │
│                 │    │                     │    │                 │
│ ┌─────────────┐ │    │ ┌─────────────────┐ │    │ ┌─────────────┐ │
│ │ Substreams  │─┼────┼►│   Substreams    │ │    │ │   Deploy    │ │
│ │    CLI      │ │    │ │   (port 9000)   │ │    │ │  Programs   │ │
│ └─────────────┘ │    │ └─────────────────┘ │    │ └─────────────┘ │
└─────────────────┘    │          │          │    └─────────────────┘
                       │ ┌─────────────────┐ │
                       │ │     Solana      │ │
                       │ │   (port 8899)   │ │
                       │ └─────────────────┘ │
                       └─────────────────────┘
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
      - --bind-address=0.0.0.0
      - --rpc-port=8899
      - --faucet-sol=1000000
    ports:
      - "8899:8899"
      - "8900:8900"
    volumes:
      - solana_data:/root/.config/solana/test-ledger
    networks:
      - solana_network

  firehose:
    image: ghcr.io/streamingfast/firehose-solana:v1.1.4
    container_name: firehose-solana
    entrypoint: ["/app/firecore"]
    command:
      - start
      - reader-node,merger,relayer,firehose,substreams-tier1,substreams-tier2
      - --config-file=
      - --log-format=text
      - --log-to-file=false
      - --data-dir=/data
      - --common-first-streamable-block=0
      - --advertise-block-id-encoding=base58
      - --advertise-chain-name=local-solana
      - --reader-node-path=/app/firesol
      - --reader-node-arguments=fetch rpc 0 --endpoints=http://solana-node:8899 --state-dir=/data/reader-state
      - --firehose-grpc-listen-addr=:8089
      - --substreams-tier1-grpc-listen-addr=:9000
      - --substreams-tier1-block-type=sf.solana.type.v1.Block
    ports:
      - "8089:8089"
      - "9000:9000"
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
NAME                   IMAGE                                          COMMAND                  SERVICE       CREATED          STATUS                           PORTS
firehose-solana        ghcr.io/streamingfast/firehose-solana:v1.1.4   "/app/firecore start…"   firehose      2 minutes ago    Up 2 minutes (healthy)           0.0.0.0:8089->8089/tcp, 0.0.0.0:9000->9000/tcp
solana-dev-validator   ghcr.io/beeman/solana-test-validator:2.2.15    "solana-test-validat…"   solana-node   2 minutes ago    Up 2 minutes                     0.0.0.0:8899-8900->8899-8900/tcp
```

### 2. Test Substreams Connectivity

Test Substreams Tier1 gRPC connectivity:

```bash
substreams run -e localhost:9000 --plaintext common@v0.1.0 -o clock -s -1
```

Expected output:
```text
Writing clock information only (no data)
----------- BLOCK #25 (7TjZKsW6nhKtS7UrJvv5eS7u6NzVTpFcM97wc7TBPgPW) age=3.301572s ---------------
...
```

{% hint style="success" %}
If all validation commands succeed, your environment is ready!
{% endhint %}

## Deploy Counter Program with Anchor

### 1. Install Required Tools

Anchor installation requires several prerequisites. Before proceeding, familiarize yourself with the [Anchor installation requirements](https://www.anchor-lang.com/docs/installation).

Install AVM (Anchor Version Manager):
```bash
cargo install --git https://github.com/coral-xyz/anchor avm --force
```

Install and use the latest Anchor version:
```bash
avm install latest
avm use latest
```

Verify installation:
```bash
anchor --version
# Expected output (latest version):
# anchor-cli 0.30.1

solana --version
# Expected output (version 2.x or higher):
# solana-cli 2.3.13 (src:5466f459; feat:2142755730, client:Agave)
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
cd counter
```

### 4. Configure Anchor

The `anchor init` command creates a default `Anchor.toml` file. Verify the configuration matches your local setup:

```toml
[provider]
cluster = "localnet"
wallet = "~/.config/solana/id.json"
```

The cluster should be set to "localnet" (lowercase) and the wallet path should match the keypair you created in the previous step.

### 5. Review Generated Program

The `anchor init counter` command created a basic program for us. The generated program source is located at `programs/counter/src/lib.rs`:

```rust
// File: programs/counter/src/lib.rs
use anchor_lang::prelude::*;

declare_id!("4xFCXVK9eym8DoPwgn9Z6gHqjmWk8B7bZimpDgP7cvNs");

#[program]
pub mod counter {
    use super::*;

    pub fn initialize(ctx: Context<Initialize>) -> Result<()> {
        msg!("Greetings from: {:?}", ctx.program_id);
        Ok(())
    }
}

#[derive(Accounts)]
pub struct Initialize {}
```

This simple program demonstrates the basic structure of an Anchor program with an `initialize` instruction.

### 6. Build and Deploy Program

Build the program:
```bash
anchor build
```

Deploy to your local cluster (as configured in `Anchor.toml`):
```bash
anchor deploy
```

After deployment, you'll see output similar to this:

```
Deploying cluster: http://127.0.0.1:8899
Upgrade authority: /Users/maoueh/.config/solana/id.json
Deploying program "counter"...
Program path: /private/tmp/substreams-solana-local/counter/target/deploy/counter.so...
Program Id: E2sTdp1aDaKbyh26BQkpYUH338u52fzwaiFbnym8MTk4

Signature: 29aeeGh2c4ogsKwnLjR8KoANY7rBDwonXdJzzDRF8MbQEASQerEMCSZbz591GfmZU8Uff5JHtnehT5w1zSbjq5wP

Waiting for program E2sTdp1aDaKbyh26BQkpYUH338u52fzwaiFbnym8MTk4 to be confirmed...
Program confirmed on-chain
Idl data length: 211 bytes
Step 0/211
Idl account created: G5GLdJwfDagZ7jfWjDZPBDwFNmZWtsY6WA3Lgbw6txmF
Deploy success
```

Export the Program ID and IDL account from the deployment output:

```bash
export PROGRAM=<PROGRAM_ID>
export IDL=<IDL_ACCOUNT>
```

Replace `<PROGRAM_ID>` with the address from the `Program Id:` line (e.g., `E2sTdp1aDaKbyh26BQkpYUH338u52fzwaiFbnym8MTk4`) and `<IDL_ACCOUNT>` with the address from the `Idl account created:` line (e.g., `G5GLdJwfDagZ7jfWjDZPBDwFNmZWtsY6WA3Lgbw6txmF`).

### 7. Verify Deployment

```bash
# Check program account
solana program show $PROGRAM

# Check if program is deployed
solana account $PROGRAM
```

## Interact with the Counter Program

Now that the program is deployed, let's interact with it to generate some on-chain activity by calling the initialize instruction.

Start the Anchor shell:

```bash
anchor shell
```

In the Anchor shell, run the following commands to call the initialize instruction:

```javascript
const program = anchor.workspace.Counter
await program.methods.initialize().accounts({}).signers([]).rpc()
```

You should see a transaction signature returned, indicating the initialize instruction was successfully called.

Exit the shell:

```javascript
.exit
```

This will create transactions on the local Solana validator that you can later observe when running your Substreams module.

## Create Substreams Module

Navigate back to the parent directory:

```bash
cd ..
```

Initialize the Substreams module:

```bash
substreams init
```

Follow the interactive prompts:
* **Chosen protocol**: `Solana`
* **Chosen generator**: `sol-anchor-beta`
* **Please enter the project name**: `counter`
* **Please select the chain**: `Solana Devnet`
* **How do you want to provide the JSON IDL?**: `JSON in a local file`
* **Input the full path of your JSON IDL in your filesystem**: `./counter/target/idl/counter.json`
* **Do you want to proceed with this IDL?**: `Yes`
* **At what block do you want to start indexing data?**: `0`
* **How would you like to consume the Substreams?**: `To Postgres`
* **In which directory do you want to download the project?**: `./substreams`

{% hint style="info" %}
The IDL file at `counter/target/idl/counter.json` was automatically generated by Anchor during the build process. We reference this file when initializing the Substreams module.
{% endhint %}

This will generate the basic Substreams module structure with the necessary configuration for tracking your Counter program.

### 2. Build and Test Substreams

```bash
cd substreams
substreams build
substreams run -e localhost:9000 --plaintext my-project-v0.1.0.spkg map_program_data -s -1
```

{% hint style="note" %}
Look for the block where you called the initialize instruction as it's the one that will contain some actual data. You can scan a specific range using `-s <BLOCK_NUMBER> -t +10` to scan 10 blocks starting from a specific block.

You can also leave the substreams run command running and open another terminal to run `anchor shell` and execute `await anchor.workspace.Counter.methods.initialize().accounts({}).signers([]).rpc()` to call the initialize instruction and see the events appear live in your Substreams output.
{% endhint %}

You should see the Counter program events from your deployment and interactions!

## Cleanup

Congratulations! You've completed the tutorial and have a working local Anchor development environment for Substreams.

When you're done, you can clean up the Docker environment with:

```bash
docker compose down --volumes
```

This will stop all containers and remove all data, allowing you to start fresh if needed.

## Troubleshooting

For common issues with Docker Compose, RPC connectivity, and Substreams, see the [Local Development Troubleshooting](../../generic/local-development/troubleshooting.md) guide.

## Next Steps

Now that you have a working local Solana development environment:

1. **Try Other Platforms** - Explore [HardHat](../../evm/local-development/hardhat.md) or [Foundry](../../evm/local-development/foundry.md) local development
2. **Advanced Substreams** - Learn about [stores, modules, and data transformations](../../generic/using-rust-proto.md)
3. **Consuming Substreams** - Connect to [databases](../../../sinks/sql/sql.md) or [streaming platforms](../../../sinks/stream/stream.md)
4. **Production Deployment** - Move to [production endpoints](../../../../references/chains-and-endpoints.md)

## Additional Resources

- [Anchor Documentation](https://www.anchor-lang.com/docs)
- [Substreams Solana Reference](https://github.com/streamingfast/substreams-solana)
- [Substreams CLI Reference](../../../../references/cli/command-line-interface.md)
- [Creating Protobuf Schemas](../../generic/creating-protobuf-schemas.md)
