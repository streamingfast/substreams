# Versioned Transactions (v0 and v1)

Solana introduced [versioned transactions](https://solana.com/docs/core/transactions/versioned-transactions) to let a transaction reference more accounts than fit in a legacy transaction, using [Address Lookup Tables (ALTs)](https://solana.com/news/transaction-v1-and-the-alt-trade-off). Legacy transactions and v0 transactions have been available for a while; v1 transactions are the newest addition.

This page explains what changed on the Substreams side, and who needs to make a change to keep consuming this data.

## What changed

Solana blocks contain transactions tagged with a `version` and a `versioned` flag. A v1 transaction also carries a `transaction_config` field:

```json
{
  "transaction_config": {
    "compute_unit_limit": 1350000,
    "heap_size": 256000,
    "loaded_accounts_data_size_limit": 2162688
  },
  "version": 1,
  "versioned": true
}
```

A v0 transaction looks the same, minus `transaction_config`:

```json
{
  "version": 0,
  "versioned": true
}
```

The Solana Firehose protobuf models expose these fields on `sf.solana.type.v1.Message`. See the [protobuf reference](https://buf.build/streamingfast/firehose-solana/docs/main%3Asf.solana.type.v1#sf.solana.type.v1.Message) for the full schema.

## Do you need to change anything?

For most Substreams users, no. Existing Substreams packages keep working against v0 and v1 transactions without any change, the same way they already work against legacy transactions.

You only need to update your Substreams package if you want to read the new fields, for example to:

- Inspect `transaction_config` (compute unit limit, heap size, loaded accounts data size limit).
- Branch your logic on whether a transaction is legacy, v0, or v1.

To do that:

1. In your Rust module's `Cargo.toml`, bump the `substreams-solana` dependency to [v0.15.1](https://crates.io/crates/substreams-solana) or later, which decodes these fields.
2. If your manifest vendors the `sf.solana.type.v1` proto files directly instead of relying on the crate, replace them with the versions linked in [Protobuf reference](https://buf.build/streamingfast/firehose-solana/docs/main%3Asf.solana.type.v1#sf.solana.type.v1.Message) and rerun `substreams protogen` (see [Protobuf](../generic/creating-protobuf-schemas.md)) to regenerate the Rust bindings.
3. Rebuild your module and redeploy the resulting `.spkg`.

{% hint style="info" %}
If you consume Solana data through the Solana RPC API instead of Substreams, check the `maxSupportedTransactionVersion` parameter on your `getTransaction`, `getBlock`, or `blockSubscribe` requests. If it's unset or too low, the RPC call fails outright on a v1 transaction (for example, `getTransaction` returns error `-32015`, and one v1 transaction in a block fails the entire `getBlock` response) rather than silently omitting it.
{% endhint %}

## Related resources

- [Solana: Versioned Transactions](https://solana.com/docs/core/transactions/versioned-transactions)
- [Solana: Transaction v1 and the ALT trade-off](https://solana.com/news/transaction-v1-and-the-alt-trade-off)
- [`sf.solana.type.v1.Message` protobuf reference](https://buf.build/streamingfast/firehose-solana/docs/main%3Asf.solana.type.v1#sf.solana.type.v1.Message)
