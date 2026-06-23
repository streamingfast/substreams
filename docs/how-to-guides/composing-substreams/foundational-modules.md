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

### Solana
Solana foundational modules offer:

- **Instructions**: Parsed and filtered program instructions
- **Account Changes**: Track state changes across Solana accounts
- **Token Programs**: SPL token transfers and account modifications

### Cosmos Ecosystem
Cosmos-compatible chains include modules for:

- **Messages and Events**: Cosmos SDK message parsing and event extraction
- **Validator Operations**: Staking, delegation, and governance activities
- **IBC Transfers**: Inter-blockchain communication tracking
- **Chain-Specific Features**: Modules tailored for Injective, Osmosis, and other Cosmos chains

### TRON
TRON foundational modules offer:

- **TRC-20 tokens**: Token transfer and balance tracking
- **Smart contract events**: Decoded contract event data

### NEAR
NEAR foundational modules include:

- **Function calls**: Parsed function call data and results
- **Account state modifications**: Track account state changes

### Antelope
Antelope foundational modules support:

- **EOS and other Antelope-based chains**: Transaction and action data
- **Resource management**: CPU, NET, and RAM usage tracking

## Using Foundational Modules

### Installation and Setup

To use foundational modules in your Substreams project:

1. **Add the dependency** to your `substreams.yaml`:
```yaml
imports:
  ethereum_common: ethereum_common@v0.3.3
```

2. **Reference the module** in your manifest with query expressions:
```yaml
modules:
  - name: my_custom_module
    kind: map
    inputs:
      - source: sf.ethereum.type.v2.Block
      - map: ethereum_common:filtered_logs
    output:
      type: proto:my.custom.Output
      
params:
  ethereum_common:filtered_logs: "address=0xa0b86a33e6776e1b1c4b0b8b8b8b8b8b8b8b8b8b" # lowercase is important! this is an exact string match
```

### Example: Using Ethereum Log Filtering

```rust
use substreams::prelude::*;
use substreams_ethereum::pb::eth::v2 as eth;

#[substreams::handlers::map]
fn process_logs(logs: eth::Logs) -> Result<MyOutput, substreams::errors::Error> {
    let mut output = MyOutput::default();

    for log in logs.logs {
        // Check for ERC-20 Transfer topic
        if log.topics.len() > 0 && log.topics[0] == "0xddf252ad1be2c89b69c2b068fc378daa952ba7f163c4a11628f55a4df523b3ef" {
            // Handle ERC-20 transfers
            output.transfers.push(process_transfer_log(&log));
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

## Performance Optimization with Indexes and Query Expressions

One of the biggest performance factors when consuming Substreams is the use of indexes and query expressions. These features allow the Substreams engine to skip processing entire blocks when they don't contain relevant data, dramatically improving performance.

### Using Query Expressions

Query expressions filter data at the source, ensuring that only relevant blocks are processed:

```yaml
modules:
  - name: filtered_transfers
    kind: map
    inputs:
      - map: ethereum_common:filtered_logs
    output:
      type: proto:my.transfers.Transfers
      
params:
  ethereum_common:filtered_logs: "address=0xa0b86a33e6776e1b1c4b0b8b8b8b8b8b8b8b8b8b&topic0=0xddf252ad1be2c89b69c2b068fc378daa952ba7f163c4a11628f55a4df523b3ef"
```

### Performance Benefits

- **Block Skipping**: When no logs match the query expression, the entire block is skipped
- **Reduced Processing**: Only relevant data flows through the pipeline
- **Lower Resource Usage**: Significant reduction in CPU and memory consumption
- **Faster Sync Times**: Historical data processing completes much faster

A well-designed query expression can improve performance by orders of magnitude, especially when processing historical data where many blocks may not contain relevant events.

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

- Explore the [Foundational Stores](foundational-stores/foundational-stores.md) for pre-computed historical data
- Discover [Published Packages](published-packages.md) from the community
- Learn how to [publish your own modules](../publish-package.md) for others to use
