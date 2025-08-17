# Logging, Debugging and Testing Reference

In this reference, you will learn how to log arbitrary inputs in your Substreams execution, and how to create unit tests for your modules.

## Using logs in Substreams

It is possible to log arbitrary strings to the standard output of the Substreams execution. You will see these logs when running your Substreams using `substreams gui` or `substreams run` commands.

**NOTE:** When using the `substreams gui` command, use the `L` key to toggle logs.

To log something in Rust, use one of the following functions:

```rust
#[substreams::handlers::map]
fn map_program_data(blk: Block) -> Data {
    substreams::log::println("My log")
}
```

OR

```rust
#[substreams::handlers::map]
fn map_program_data(blk: Block) -> Data {
    substreams::log::debug!("My log")
}
```

OR

```rust
#[substreams::handlers::map]
fn map_program_data(blk: Block) -> Data {
    substreams::log::info!("My log")
}
```

## Unit Testing

Testing a Substreams does not differ much from testing a standard Rust application. As an example, you can refer to the tests in the [Ethereum Foundational Modules Substreams](https://github.com/streamingfast/substreams-foundational-modules/tree/develop/ethereum-common). Let's take a look at the [filtered_events test](https://github.com/streamingfast/substreams-foundational-modules/blob/develop/ethereum-common/src/events.rs#L95).

1. The definition of the function is a standard Substreams module, which needs the `#[substreams::handlers::map]` macro. Because of the macro, the _real_ signature of the function is more complex than just a couple of input parameters. Therefore, to facilitate the testing, all the logic is contained inside the `_filtered_events` function.

```rust
#[substreams::handlers::map]
fn filtered_events(query: String, events: Events) -> Result<Events, Error> {
    _filtered_events(query, events)
}
```

2. The `_filtered_events` function, which contains the real logic, is just filtering the events (`events: Events`) matching the filter provided in the query (`query: String`).

```rust
/// _filtered_events is equal to [filtered_events] but exists only for unit testing purposes.
fn _filtered_events(query: String, mut events: Events) -> Result<Events, Error> {
    let matcher: substreams::ExprMatcher<'_> = substreams::expr_matcher(&query);

    events.events.retain(|event| {
        let keys = evt_keys(event.log.as_ref().unwrap());
        let keys = keys.iter().map(|k| k.as_str()).collect::<Vec<&str>>();

        matcher.matches_keys(&keys)
    });

    Ok(events)
}
```

3. The `filtered_events` test is split into three parts:
    * **Given:** create the inputs of the function. In this case, we are reading a full Ethereum Block from a base-64-encoded file. Later in this doc, you will learn how to generate this file.
    * **When:** the actual execution of the function. The tests will filter all the events with `address=0x5acc84a3e955bdd76467d3348077d003f00ffb97` because we know this event is contained in the input Block.
    * **Expect:** there are your assertions: the number of events returned must be greater than 0; all the events returned must have the `0x5acc84a3e955bdd76467d3348077d003f00ffb97` address.


```rust
#[test]
fn test_filtered_events() {
    // Given
    let block: Block =
        testing::read_block("./src/testdata/ethereum_mainnet_10500500.binpb.base64");

    // When
    let result = _filtered_events(
        "evt_addr:0x5acc84a3e955bdd76467d3348077d003f00ffb97".to_owned(),
        _all_events(block).unwrap(),
    )
    .expect("Failed to execute function");

    // Expect
    assert!(result.events.len() > 0);
    result.events.iter().for_each(|e| {
        let address: &Vec<u8> = &e.log.as_ref().unwrap().address;

        assert_eq!(
            Hex::encode(address),
            "5acc84a3e955bdd76467d3348077d003f00ffb97"
        );
    });
}
```

### Generating the Input of the Test

Usually, the input of a Substreams module must be the original source `Block` of the blockchain (or any of the related structures, such as `Events`, `Calls`...). You can get the raw `Block` of a blockchain using the [firecore](https://github.com/streamingfast/firehose-core/releases) tool (download the binary from the GitHub releases section).

The following example uses `firecore` to get the `5300300` Block of Ethereum Mainnet in base64-encoded file.

```bash
firecore tools firehose-single-block-client mainnet.eth.streamingfast.io:443 5300300 --output=bytes --bytes-encoding=base64  > /tmp/ethereum-mainnet-block-1.binpb.base64
```

You can include the base64 file in your Rust tests and use the `Block` Protobuf to [decode the data](https://github.com/streamingfast/substreams-foundational-modules/blob/develop/testing/src/lib.rs#L6).