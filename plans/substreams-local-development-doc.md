# Substreams Local Development Guides Implementation Plan

## Overview
Create comprehensive local development guides for Substreams covering three platforms:
1. Ethereum with HardHat
2. Ethereum with Foundry
3. Solana with Anchor

Each guide provides a complete Docker Compose environment with blockchain node + Firehose + example contract/program + Substreams module, all validated by automated testing.

## File Structure

```
docs/how-to-guides/develop-your-own-substreams/generic/
└── local-development/
    ├── README.md                    # Overview/index page
    ├── ethereum-hardhat.md          # Complete Ethereum + HardHat guide
    ├── ethereum-foundry.md          # Complete Ethereum + Foundry guide
    ├── solana.md                    # Complete Solana + Anchor guide
    └── examples/
        ├── ethereum-counter/
        │   ├── docker-compose.yml
        │   ├── contracts/Counter.sol
        │   ├── hardhat/              # HardHat config & scripts
        │   ├── foundry/              # Foundry config & scripts
        │   └── substreams/           # Substreams module
        └── solana-counter/
            ├── docker-compose.yml
            ├── programs/counter/     # Anchor program
            ├── Anchor.toml
            ├── tests/                # Deployment tests
            └── substreams/           # Substreams module
```

## Documentation Updates

### SUMMARY.md Changes
**File:** `docs/SUMMARY.md`

**Current line 45:**
```markdown
  * [General](how-to-guides/develop-your-own-substreams/generic/using-rust-proto.md)
```

**Replace with:**
```markdown
  * General
    * [Local Development](how-to-guides/develop-your-own-substreams/generic/local-development/README.md)
      * [Ethereum (HardHat)](how-to-guides/develop-your-own-substreams/generic/local-development/ethereum-hardhat.md)
      * [Ethereum (Foundry)](how-to-guides/develop-your-own-substreams/generic/local-development/ethereum-foundry.md)
      * [Solana (Anchor)](how-to-guides/develop-your-own-substreams/generic/local-development/solana.md)
    * [Using Rust & Protobuf](how-to-guides/develop-your-own-substreams/generic/using-rust-proto.md)
```

## Guide Structure

### README.md (Overview)
- Introduction to local development benefits
- Prerequisites: Docker, Docker Compose, CLI tools
- Links to three platform-specific guides
- When to use local vs production endpoints

### ethereum-hardhat.md (Complete Guide)

**Sections:**
1. **Introduction** - What you'll build, use cases, 15-20 min estimate
2. **Prerequisites** - Docker 20.10+, Node 18+, Substreams CLI, Rust
3. **Architecture Overview** - Geth dev mode + Fireeth components, port mappings (8545, 8546, 8089)
4. **Setup Instructions**
   - Docker Compose with `ghcr.io/streamingfast/go-ethereum:geth-v1.16.7-fh3.0`
   - Geth args: `--dev --dev.period=1 --http --gcmode=archive --state.scheme=hash`
   - Fireeth modules: `reader-node,relayer,merger,firehose,substreams-tier1,substreams-tier2`
5. **Validation Commands**
   - Docker health checks
   - RPC endpoint tests (curl eth_blockNumber)
   - Firehose gRPC connectivity:
     * Using fireeth: `docker run --rm --network=host ghcr.io/streamingfast/firehose-ethereum:latest fireeth tools firehose-client localhost:8089 --plaintext -o text -- -1`
     * Using substreams: `docker run --rm --network=host ghcr.io/streamingfast/substreams:latest substreams run -e localhost:8089 --plaintext common@v0.1.0 -s -1`
   - Block streaming verification
6. **Deploy Counter Contract with HardHat**
   - Setup: `npm init`, install hardhat, `npx hardhat init`
   - Configure hardhat.config.js with local network (chainId 1337)
   - Counter.sol contract (increment/decrement with events)
   - Deployment script (deploy + 2x increment for test data)
   - Verification commands
7. **Create Substreams Module**
   - Protobuf definition (CounterEvents message)
   - substreams.yaml manifest with contract parameter
   - Cargo.toml with substreams-ethereum dependencies
   - Rust handler using Event derive macros
   - Build and test streaming
8. **Troubleshooting** - Docker issues, RPC problems, HardHat errors, Firehose connectivity, Substreams builds
9. **Next Steps** - Extend contract, advanced Substreams, testing
10. **Additional Resources** - Links to HardHat docs, Substreams references

**Key Technical Details:**
- Base image contains both fireeth and geth
- Reference: battlefield-ethereum scripts for proper geth/fireeth args
- Default HardHat account 0 private key for dev mode
- Sample contract emits Incremented/Decremented events with newCount and caller
- Substreams uses parameterized contract address

### ethereum-foundry.md (Complete Guide)

**Same structure as HardHat guide, with differences:**

**Section 6: Deploy Counter Contract with Foundry**
- Install Foundry: `curl -L https://foundry.paradigm.xyz | bash && foundryup`
- Initialize: `forge init --no-git`
- Configure foundry.toml with local RPC endpoint
- Same Counter.sol contract
- Deployment via Deploy.s.sol script using forge-std/Script.sol
- Compile: `forge build`
- Deploy: `forge script script/Deploy.s.sol --rpc-url local --broadcast --legacy`
- Verify: `cast call <ADDRESS> "getCount()(uint256)" --rpc-url local`

**Section 8.3: Foundry-Specific Troubleshooting**
- "failed to get chain id" - verify RPC, use `--chain-id 1337`
- "InvalidTransaction" - use `--legacy` flag
- "NonceTooLow" - wait for block, reset if needed

**Rest identical:** Substreams module, Docker setup, validation all same

### solana.md (Complete Guide)

**Sections:**
1. **Introduction** - Solana validator + Firehose + Anchor, 20-25 min
2. **Prerequisites** - Docker, Node 18+, Anchor CLI 0.30.1, Solana CLI 1.18.26, Rust
3. **Architecture Overview** - Solana test validator + Firehose Solana, ports (8899, 8900, 9000)
4. **Setup Instructions**
   - Docker Compose based on substreams-solana-devenv reference
   - Validator: `solana/solana:v1.18.26` with test-validator
   - Firehose: `ghcr.io/streamingfast/firehose-solana:v1.1.0`
   - Validator args: `--enable-rpc-transaction-history --enable-extended-tx-metadata-storage`
   - Firehose: `fetch rpc http://solana-validator:8899`
5. **Validation Commands**
   - Docker health checks (both services)
   - RPC health test (curl getHealth)
   - Firehose connectivity
   - Block streaming test
6. **Deploy Counter Program with Anchor**
   - Install Anchor: `cargo install avm && avm install 0.30.1`
   - Initialize: `anchor init counter --no-git`
   - Configure Anchor.toml for localnet
   - Counter program with initialize/increment/decrement instructions
   - Events: CounterInitialized, CounterIncremented, CounterDecremented
   - Configure Solana CLI: `solana config set --url http://localhost:8899`
   - Airdrop: `solana airdrop 10`
   - Build: `anchor build`
   - Deploy: `anchor deploy`
   - Test: `anchor test --skip-local-validator`
7. **Create Substreams Module**
   - Protobuf with CounterEvent (slot, tx_id, event_type, etc.)
   - substreams.yaml with program parameter
   - Cargo.toml with substreams-solana dependencies
   - Rust handler parsing Anchor event logs
   - Event discriminator matching
8. **Troubleshooting** - Docker, validator, Anchor/CLI, Firehose, Substreams
9. **Next Steps**
10. **Additional Resources** - Anchor docs, Solana Cookbook, Substreams references

**Key Technical Details:**
- Solana uses slots (different from Ethereum blocks)
- Anchor event discriminators for log parsing
- Test validator provides unlimited SOL via airdrop
- Program ID must be saved for Substreams params
- Dependencies between services via healthchecks

## Example Code Specifications

### Ethereum Counter.sol
```solidity
// Simple counter with events
contract Counter {
    uint256 private count;
    event Incremented(uint256 newCount, address caller);
    event Decremented(uint256 newCount, address caller);

    constructor(uint256 initialCount) { count = initialCount; }
    function increment() public { count += 1; emit Incremented(count, msg.sender); }
    function decrement() public { require(count > 0, "..."); count -= 1; emit Decremented(count, msg.sender); }
    function getCount() public view returns (uint256) { return count; }
}
```

### Solana Counter Program (Anchor)
```rust
#[program]
pub mod counter {
    pub fn initialize(ctx: Context<Initialize>, initial_count: u64) -> Result<()>
    pub fn increment(ctx: Context<Update>) -> Result<()>
    pub fn decrement(ctx: Context<Update>) -> Result<()>
}

#[account]
pub struct Counter {
    pub count: u64,
    pub authority: Pubkey,
}

#[event]
pub struct CounterIncremented { counter: Pubkey, new_count: u64, caller: Pubkey }
```

### Substreams Modules
- **Ethereum:** Uses `substreams-ethereum` Event derive macros for ABI decoding
- **Solana:** Parses log messages for Anchor event discriminators
- Both: Parameterized by contract/program address
- Output: CounterEvents protobuf with array of CounterEvent

## Validation Strategy (Remote Agent Testing)

Each guide must pass these automated tests:

### 1. Docker Startup
- `docker-compose up -d` succeeds
- All containers reach "running" state (60s timeout)
- All containers show "healthy" (120s timeout)

### 2. RPC Connectivity
- Ethereum: `curl eth_blockNumber` returns valid JSON
- Solana: `curl getHealth` returns "ok"
- Timeout: 5 seconds

### 3. Firehose Connectivity
- Ethereum: `docker run --rm --network=host ghcr.io/streamingfast/firehose-ethereum:latest fireeth tools firehose-client localhost:8089 --plaintext -o text -- -1` succeeds OR
  `docker run --rm --network=host ghcr.io/streamingfast/substreams:latest substreams run -e localhost:8089 --plaintext common@v0.1.0 -s -1` succeeds
- Solana: `docker run --rm --network=host ghcr.io/streamingfast/firehose-solana:latest firecore tools firehose-client localhost:9000 --plaintext -o text -- -1` succeeds OR
  `docker run --rm --network=host ghcr.io/streamingfast/substreams:latest substreams run -e localhost:9000 --plaintext common@v0.1.0 -s -1` succeeds
- Timeout: 10 seconds

### 4. Contract/Program Build
- Compile commands exit 0
- Output artifacts created

### 5. Deployment
- Deploy commands exit 0
- Address/Program ID captured
- Verification commands confirm deployment

### 6. Transaction Generation
- Deployment scripts call increment functions
- Transactions confirmed on-chain

### 7. Substreams Build & Test
- `substreams protogen` generates code
- `cargo build --target wasm32-unknown-unknown --release` succeeds
- `substreams gui` streams events successfully
- At least 2 Incremented events visible
- Event data matches expected values (count 1, 2)

### 8. End-to-End
- Additional transaction created
- Substreams re-run shows new event

## Version Pinning

**Ethereum:**
- Docker image: `ghcr.io/streamingfast/go-ethereum:geth-v1.16.7-fh3.0`
- Solidity: 0.8.20
- HardHat: Latest via @nomicfoundation/hardhat-toolbox
- Foundry: Latest via foundryup

**Solana:**
- Validator: `solana/solana:v1.18.26`
- Firehose: `ghcr.io/streamingfast/firehose-solana:v1.1.0`
- Anchor CLI: 0.30.1
- Solana CLI: 1.18.26

**Substreams:**
- CLI: v1.7.0+ (document latest stable)
- substreams crate: 0.6
- substreams-ethereum: 0.10
- substreams-solana: 0.8

## Remote Agent Requirements

### Environment
- Docker Engine 20.10+ with 4GB RAM, 20GB disk
- Docker Compose v2.0+
- Ports available: 8089, 8545, 8546 (Ethereum); 8899, 8900, 9000 (Solana)

### Tools - Ethereum Guides
- Node.js 18.x or 20.x
- HardHat guide: npm/yarn
- Foundry guide: Foundry toolkit (forge, cast)

### Tools - Solana Guide
- Node.js 18.x or 20.x
- Rust 1.75+
- Anchor CLI 0.30.1
- Solana CLI 1.18.26
- Yarn

### Tools - All Guides
- Substreams CLI (latest stable)
- Rust with wasm32-unknown-unknown target
- curl, git, bash
- Optional: grpcurl

## Documentation Style

Follow existing Substreams documentation patterns:
- YAML front matter (title, description)
- GitBook hint boxes (info, warning, success, note)
- Clear H2/H3 hierarchy
- Code blocks with language tags
- Numbered step-by-step sections
- Command verification after each step
- "SAVE THIS ADDRESS/ID" warnings in bold
- Troubleshooting sections with problem/solution format

## GitBook Hints to Include

- **Warning:** "These Docker configurations are for local development only - never use in production"
- **Info:** "Save your deployed contract address - you'll need it in multiple places"
- **Success:** "If all validation commands succeed, your environment is ready!"
- **Note:** "Dev mode produces blocks every 1 second when transactions are pending" (Ethereum)
- **Note:** "Solana uses slots (400ms) - multiple slots can be in one block" (Solana)
- **Warning:** "Local validator provides unlimited SOL - production networks require real SOL" (Solana)

## Common Pitfalls & Preventions

### Docker Issues
- Health checks with adequate start_period
- Explicit port mappings and network configs
- Volume cleanup commands documented
- Mac-specific networking notes

### Block Production
- Ensure dev mode actively produces blocks
- Transaction generation in deployment scripts
- Verification that blocks are increasing

### Account Management
- Document default accounts/keys
- Show balance checks before deployment
- Explicit airdrop commands (Solana)

### Address/ID Tracking
- Bold save instructions after deployment
- Show exact config locations to update
- Verification commands to confirm correctness

### Build Issues
- Document wasm32 target installation
- Exact protogen exclude-paths
- Version compatibility notes

## Critical Files to Create

1. **SUMMARY.md** - Update navigation structure
2. **local-development/README.md** - Overview page
3. **local-development/ethereum-hardhat.md** - Full HardHat guide (~10 sections)
4. **local-development/ethereum-foundry.md** - Full Foundry guide (~10 sections)
5. **local-development/solana.md** - Full Solana guide (~10 sections)
6. **examples/ethereum-counter/docker-compose.yml** - Working Ethereum Docker setup
7. **examples/ethereum-counter/contracts/Counter.sol** - Sample contract
8. **examples/ethereum-counter/hardhat/** - HardHat config & scripts
9. **examples/ethereum-counter/foundry/** - Foundry config & scripts
10. **examples/ethereum-counter/substreams/** - Complete Substreams module
11. **examples/solana-counter/docker-compose.yml** - Working Solana Docker setup
12. **examples/solana-counter/programs/counter/src/lib.rs** - Anchor program
13. **examples/solana-counter/Anchor.toml** - Anchor config
14. **examples/solana-counter/tests/** - Deployment tests
15. **examples/solana-counter/substreams/** - Complete Substreams module

## Implementation Notes for Remote Agent

**CRITICAL:** Every command in the guides must be executed by the remote agent to verify:
1. Commands run without errors
2. Expected output is produced
3. Subsequent steps can proceed
4. Full end-to-end flow works

**Testing approach:**
- Start with Docker Compose validation
- Proceed through deployment step-by-step
- Verify each command's output before continuing
- Capture contract addresses/program IDs for later steps
- Test Substreams with actual captured addresses
- Document any command failures or unexpected behavior
- Update troubleshooting sections based on encountered issues

**Documentation quality checks:**
- All code blocks are copy-pasteable
- All file paths are correct and consistent
- All port numbers match between services
- All version numbers are accurate
- Links to other documentation resolve correctly
- GitBook syntax renders properly

## Success Criteria

A guide is complete when:
1. ✅ All sections written with proper formatting
2. ✅ All example code created and tested
3. ✅ Docker Compose starts successfully
4. ✅ Contract/program deploys successfully
5. ✅ Transactions generate events on-chain
6. ✅ Substreams builds without errors
7. ✅ Substreams streams events with correct data
8. ✅ Troubleshooting covers encountered issues
9. ✅ All commands verified by remote agent
10. ✅ End-to-end flow completes in documented timeframe

## References

**External:**
- battlefield-ethereum scripts: geth/fireeth configuration patterns
- substreams-solana-devenv: Solana Docker Compose reference

**Internal Docs:**
- `docs/how-to-guides/cli/installing-the-cli.md` - Substreams CLI installation
- `docs/references/cli/command-line-interface.md` - CLI reference
- `docs/how-to-guides/develop-your-own-substreams/generic/creating-protobuf-schemas.md` - Protobuf schemas
- `docs/references/devcontainer-ref.md` - Alternative dev environment
