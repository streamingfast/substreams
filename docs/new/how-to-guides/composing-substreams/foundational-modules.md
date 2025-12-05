# Foundational Modules

Foundational modules are pre-built Substreams modules that provide common blockchain data transformations and extractions. These modules serve as building blocks for more complex data processing pipelines, offering standardized ways to access and transform blockchain data across different networks.

## What Are Foundational Modules?

Foundational modules are production-ready Substreams modules that handle common blockchain data patterns. They provide:

- **Pre-transformed blockchain models**: Clean, structured data formats that are ready for consumption
- **Standardized interfaces**: Consistent APIs across different blockchain networks
- **Optimized performance**: Efficient data processing with minimal computational overhead
- **Multi-chain support**: Modules available for Ethereum, Solana, Cosmos, and other major blockchains

## Key Chains and Capabilities

### Ethereum
The Ethereum foundational modules provide comprehensive access to:

- **Events and Logs**: Filtered and decoded smart contract events
- **Transactions**: Detailed transaction data with receipt information
- **Blocks**: Complete block data with metadata
- **Token Transfers**: ERC-20, ERC-721, and ERC-1155 token movement tracking
- **DeFi Protocols**: Pre-built modules for popular DeFi protocols like Uniswap, Aave, and Compound

### Solana
Solana foundational modules offer:

- **Instructions**: Parsed and filtered program instructions
- **Account Changes**: Track state changes across Solana accounts
- **Token Programs**: SPL token transfers and account modifications
- **Program-Specific Data**: Modules for popular Solana programs like Serum, Raydium, and Jupiter

### Cosmos Ecosystem
Cosmos-compatible chains include modules for:

- **Messages and Events**: Cosmos SDK message parsing and event extraction
- **Validator Operations**: Staking, delegation, and governance activities
- **IBC Transfers**: Inter-blockchain communication tracking
- **Chain-Specific Features**: Modules tailored for Injective, Osmosis, and other Cosmos chains

### Other Supported Chains
Additional foundational modules are available for:

- **Starknet**: Cairo program interactions and state changes
- **TRON**: TRC-20 tokens and smart contract events
- **NEAR**: Function calls and account state modifications
- **Antelope**: EOS and other Antelope-based chain data

## Using Foundational Modules

### Installation and Setup

To use foundational modules in your Substreams project:

1. **Add the dependency** to your `substreams.yaml`:
```yaml
imports:
  ethereum: https://github.com/streamingfast/substreams-foundational-modules/releases/download/ethereum-v0.1.0/ethereum-v0.1.0.spkg
```

2. **Reference the module** in your manifest:
```yaml
modules:
  - name: my_custom_module
    kind: map
    inputs:
      - source: sf.ethereum.type.v2.Block
      - map: ethereum:filtered_transactions
    output:
      type: proto:my.custom.Output
```

### Example: Using Ethereum Event Filtering

```rust
use substreams::prelude::*;
use substreams_ethereum::pb::eth::v2 as eth;

#[substreams::handlers::map]
fn process_events(events: eth::Events) -> Result<MyOutput, substreams::errors::Error> {
    let mut output = MyOutput::default();
    
    for event in events.events {
        // Process pre-filtered and decoded events
        match event.name.as_str() {
            "Transfer" => {
                // Handle ERC-20 transfers
                output.transfers.push(process_transfer(event));
            }
            "Swap" => {
                // Handle DEX swaps
                output.swaps.push(process_swap(event));
            }
            _ => {}
        }
    }
    
    Ok(output)
}
```

## Available Module Categories

### Data Extraction Modules
- **Block processors**: Extract and normalize block-level data
- **Transaction filters**: Filter transactions by various criteria
- **Event decoders**: Decode smart contract events with ABI information
- **Log processors**: Parse and structure blockchain logs

### Data Transformation Modules
- **Price feeds**: Convert token amounts to USD values
- **Address resolvers**: Map addresses to human-readable names
- **Time converters**: Convert block numbers to timestamps
- **Data aggregators**: Combine data from multiple sources

### Protocol-Specific Modules
- **DeFi protocols**: Uniswap, SushiSwap, Curve, Aave, Compound
- **NFT marketplaces**: OpenSea, LooksRare, X2Y2
- **Gaming protocols**: Axie Infinity, The Sandbox
- **Infrastructure**: ENS, Chainlink, The Graph

## Benefits of Using Foundational Modules

### Development Speed
- **Rapid prototyping**: Get started quickly with proven modules
- **Reduced boilerplate**: Focus on business logic rather than data extraction
- **Tested components**: Use modules that have been battle-tested in production

### Data Quality
- **Consistent formats**: Standardized data structures across different use cases
- **Validated logic**: Modules are thoroughly tested and validated
- **Community feedback**: Benefit from community contributions and bug reports

### Maintenance
- **Automatic updates**: Receive improvements and bug fixes automatically
- **Security patches**: Stay protected with timely security updates
- **Performance optimizations**: Benefit from ongoing performance improvements

## Contributing to Foundational Modules

The foundational modules are open source and welcome community contributions:

1. **Report Issues**: Submit bug reports and feature requests
2. **Contribute Code**: Add new modules or improve existing ones
3. **Documentation**: Help improve module documentation and examples
4. **Testing**: Contribute test cases and validation scenarios

Visit the [Substreams Foundational Modules repository](https://github.com/streamingfast/substreams-foundational-modules) to get started with contributions.

## Next Steps

- Explore the [Foundational Stores](foundational-stores.md) for pre-computed historical data
- Discover [Published Packages](published-packages.md) from the community
- Learn how to [publish your own modules](../publish-package.md) for others to use
