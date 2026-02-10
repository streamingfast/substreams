# Logging and Debugging Reference

In this reference, you will learn how to log arbitrary inputs in your Substreams execution for debugging purposes.

For testing your Substreams modules, see the [Testing Reference](./testing.md).

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