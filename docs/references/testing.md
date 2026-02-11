# Testing Reference

This reference covers testing strategies for Substreams applications, from unit testing individual handlers to integration testing complete modules with real blockchain data.

## Why Testing Matters

Blockchain data is immutable and mistakes are permanent. Testing is critical because:

- Small bugs amplify across millions of blocks
- Complex transformations introduce edge cases
- DeFi applications depend on data accuracy
- Parallel execution can create race conditions

## Unit Testing

The `substreams::testing` module provides utilities for testing map handlers directly without WASM compilation overhead.

### The `map!` Macro

The `substreams::testing::map!` macro allows you to invoke map handlers directly in unit tests:

```rust
use substreams::errors::Error;
use substreams_ethereum::pb::eth::v2::Block;

mod pb;
use pb::myproject::{Events, Event};

#[substreams::handlers::map]
pub fn map_events(block: Block) -> Result<Events, Error> {
    let events: Vec<Event> = block.logs()
        .filter(|log| !log.topics.is_empty())
        .filter_map(|log| parse_event(log).ok())
        .collect();

    Ok(Events { events })
}

#[substreams::handlers::map]
pub fn filter_events(
    event_type: String,
    events: Events,
) -> Result<Events, Error> {
    let filtered: Vec<Event> = events.events
        .into_iter()
        .filter(|e| e.event_type == event_type)
        .collect();

    Ok(Events { events: filtered })
}

#[cfg(test)]
mod tests {
    use super::*;
    use substreams::testing;

    #[test]
    fn test_map_events() {
        // Arrange
        let block = create_test_block();

        // Act
        let result = testing::map!(map_events(block)).unwrap();

        // Assert
        assert!(!result.events.is_empty());
    }

    #[test]
    fn test_map_events_empty_block() {
        let empty_block = Block::default();

        let result = testing::map!(map_events(empty_block)).unwrap();

        assert!(result.events.is_empty());
    }

    #[test]
    fn test_chained_handlers() {
        let block = create_test_block();

        // Chain multiple handlers together
        let all_events = testing::map!(map_events(block)).unwrap();
        let transfers = testing::map!(filter_events("transfer".to_string(), all_events)).unwrap();

        assert!(transfers.events.iter().all(|e| e.event_type == "transfer"));
    }

    fn create_test_block() -> Block {
        // Create a test block with sample data
        Block {
            number: 17000000,
            // ... populate with test data
            ..Default::default()
        }
    }
}
```

### The `clock()` Function

For modules that depend on block time or need clock information:

```rust
use substreams::testing;

#[test]
fn test_time_dependent_logic() {
    let clock = testing::clock();

    // Use clock in time-dependent tests
    let result = process_with_time(data, clock);

    assert!(result.is_ok());
}
```

### Opting Out of Testable Generation

Map handlers automatically generate testable functions that the `map!` macro uses. In rare cases where you don't want this behavior:

```rust
#[substreams::handlers::map(no_testable)]
pub fn my_handler(block: Block) -> Result<Output, Error> {
    // This handler won't generate a testable function
    // You'll need to extract logic to separate functions for testing
}
```

## Test Data

### Using Real Blockchain Data

Use the [firecore](https://github.com/streamingfast/firehose-core/releases) tool to extract real blocks for testing:

```bash
firecore tools firehose-single-block-client mainnet.eth.streamingfast.io:443 17000000 \
    --output=bytes \
    --bytes-encoding=base64 \
    > ./src/testdata/ethereum_mainnet_17000000.binpb.base64
```

Load the block in your tests:

```rust
use prost::Message;
use std::fs;
use substreams_ethereum::pb::eth::v2::Block;

fn load_test_block(path: &str) -> Block {
    let base64_data = fs::read_to_string(path).expect("Failed to read test file");
    let bytes = base64::decode(base64_data.trim()).expect("Failed to decode base64");
    Block::decode(bytes.as_slice()).expect("Failed to decode protobuf")
}

#[test]
fn test_with_real_block() {
    let block = load_test_block("./src/testdata/ethereum_mainnet_17000000.binpb.base64");

    let result = testing::map!(map_events(block)).unwrap();

    // Assertions based on known block contents
    assert!(result.events.len() > 0);
}
```

### Constructing Test Blocks

For controlled testing scenarios, construct blocks programmatically:

```rust
use substreams_ethereum::pb::eth::v2::{Block, TransactionTrace, TransactionReceipt, Log};

fn create_block_with_transfer() -> Block {
    Block {
        number: 17000000,
        hash: hex::decode("abc123...").unwrap(),
        transaction_traces: vec![
            TransactionTrace {
                hash: hex::decode("def456...").unwrap(),
                receipt: Some(TransactionReceipt {
                    logs: vec![
                        Log {
                            address: hex::decode("a0b86991c6218b36c1d19d4a2e9eb0ce3606eb48").unwrap(),
                            topics: vec![
                                // Transfer(address,address,uint256)
                                hex::decode("ddf252ad1be2c89b69c2b068fc378daa952ba7f163c4a11628f55a4df523b3ef").unwrap(),
                            ],
                            data: vec![],
                            ..Default::default()
                        }
                    ],
                    ..Default::default()
                }),
                ..Default::default()
            }
        ],
        ..Default::default()
    }
}
```

### Test Data Builder Pattern

For complex test scenarios, use a builder pattern:

```rust
struct TestBlockBuilder {
    block: Block,
}

impl TestBlockBuilder {
    fn new(number: u64) -> Self {
        Self {
            block: Block {
                number,
                ..Default::default()
            }
        }
    }

    fn with_transfer(mut self, from: &str, to: &str, amount: u64) -> Self {
        // Add transfer log to block
        self
    }

    fn build(self) -> Block {
        self.block
    }
}

#[test]
fn test_with_builder() {
    let block = TestBlockBuilder::new(17000000)
        .with_transfer("0xabc...", "0xdef...", 1000)
        .build();

    let result = testing::map!(map_events(block)).unwrap();
    assert_eq!(result.events.len(), 1);
}
```

## Integration Testing

Test complete modules with real blockchain data to verify behavior across multiple blocks.

```rust
#[test]
fn test_balance_consistency() {
    let blocks = vec![
        load_test_block("./testdata/block_17000000.binpb.base64"),
        load_test_block("./testdata/block_17000001.binpb.base64"),
        load_test_block("./testdata/block_17000002.binpb.base64"),
    ];

    let mut total_transfers = 0u64;

    for block in blocks {
        let result = testing::map!(map_transfers(block)).unwrap();
        total_transfers += result.transfers.len() as u64;
    }

    assert!(total_transfers > 0, "Expected transfers across block range");
}
```

## End-to-End Testing

Execute complete Substreams in the real execution environment using the CLI:

```bash
# Run module on a block range
substreams run -s 17000000 -t +100 map_events --network mainnet

# Validate with production mode caching
substreams run -s 17000000 -t +1000 map_events --production-mode
```

Wrap CLI commands in Rust tests for automated validation:

```rust
use std::process::Command;

#[test]
#[ignore] // Run with: cargo test -- --ignored
fn test_e2e_execution() {
    let output = Command::new("substreams")
        .args([
            "run",
            "-s", "17000000",
            "-t", "+10",
            "map_events",
            "--network", "mainnet",
            "-o", "json"
        ])
        .output()
        .expect("Failed to execute substreams");

    assert!(output.status.success());

    let stdout = String::from_utf8_lossy(&output.stdout);
    assert!(stdout.contains("events"));
}
```

## Performance Testing

### Benchmarking with Criterion

Use the [Criterion](https://crates.io/crates/criterion) crate for reliable benchmarks:

```rust
// benches/module_bench.rs
use criterion::{black_box, criterion_group, criterion_main, Criterion};

fn bench_map_events(c: &mut Criterion) {
    let block = load_heavy_block(); // Block with many transactions

    c.bench_function("map_events", |b| {
        b.iter(|| {
            testing::map!(map_events(black_box(block.clone())))
        })
    });
}

criterion_group!(benches, bench_map_events);
criterion_main!(benches);
```

### Production Mode Validation

Test with production mode to validate caching behavior and performance:

```bash
time substreams run -s 17000000 -t +1000 map_events --production-mode
```

## Testing Best Practices

### Test Structure

Follow the Given-When-Expect pattern for clarity:

```rust
#[test]
fn test_filter_events_by_address() {
    // Given: A block with multiple events
    let block = create_block_with_mixed_events();
    let target_address = "0x5acc84a3e955bdd76467d3348077d003f00ffb97";

    // When: Filtering by address
    let all_events = testing::map!(map_events(block)).unwrap();
    let filtered = testing::map!(filter_by_address(target_address.to_string(), all_events)).unwrap();

    // Expect: Only matching events returned
    assert!(filtered.events.len() > 0);
    for event in &filtered.events {
        assert_eq!(hex::encode(&event.address), target_address.trim_start_matches("0x"));
    }
}
```

### Error Scenario Testing

Test malformed data and edge cases:

```rust
#[test]
fn test_handles_empty_topics() {
    let block = create_block_with_log(Log {
        topics: vec![], // No topics
        ..Default::default()
    });

    let result = testing::map!(map_events(block)).unwrap();

    // Should handle gracefully, not panic
    assert!(result.events.is_empty());
}

#[test]
fn test_handles_invalid_address() {
    let block = create_block_with_log(Log {
        address: vec![0x00], // Invalid: too short
        ..Default::default()
    });

    let result = testing::map!(map_events(block));

    // Verify error handling
    assert!(result.is_err() || result.unwrap().events.is_empty());
}
```

### Reorganization Testing

For stores that maintain state, test chain reorganization scenarios:

```rust
#[test]
fn test_store_handles_reorg() {
    // Process blocks on main chain
    let block_a = create_block(100, "hash_a");
    let block_b = create_block(101, "hash_b");

    // Simulate reorg: different block at same height
    let block_b_prime = create_block(101, "hash_b_prime");

    // Verify store correctly handles the reorganization
    // ...
}
```

## CI/CD Integration

### GitHub Actions Example

```yaml
name: Test Substreams

on: [push, pull_request]

jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4

      - name: Install Rust
        uses: dtolnay/rust-action@stable
        with:
          targets: wasm32-unknown-unknown

      - name: Run unit tests
        run: cargo test

      - name: Check WASM compilation
        run: cargo build --target wasm32-unknown-unknown --release

      - name: Run clippy
        run: cargo clippy -- -D warnings

      - name: Check formatting
        run: cargo fmt -- --check
```

### Pre-commit Hooks

Automate quality checks before commits:

```bash
#!/bin/bash
# .git/hooks/pre-commit

cargo fmt -- --check || exit 1
cargo clippy -- -D warnings || exit 1
cargo test || exit 1
cargo build --target wasm32-unknown-unknown --release || exit 1
```

## Summary

| Test Level | Purpose | Tools |
|------------|---------|-------|
| Unit | Test individual handlers | `substreams::testing::map!` |
| Integration | Test with real blockchain data | `firecore`, test fixtures |
| End-to-End | Validate complete pipeline | `substreams` CLI |
| Performance | Benchmark and optimize | Criterion, production mode |

Start with comprehensive unit tests, add integration tests with real data, and validate with end-to-end tests. Automate everything through CI/CD.

## Cargo.toml Requirements

```toml
[dependencies]
substreams = "0.7.4"
substreams-ethereum = "0.9"  # Or appropriate chain package

[dev-dependencies]
hex = "0.4"
base64 = "0.21"
prost = "0.11"

# For benchmarks
[dev-dependencies.criterion]
version = "0.5"
features = ["html_reports"]

[[bench]]
name = "module_bench"
harness = false
```
