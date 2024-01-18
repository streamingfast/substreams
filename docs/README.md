Substreams is a powerful indexing technology, which allows you to:
- Extract data from several blockchains (Ethereum, Polygon, BNB, Solana...).
- Apply custom transformations to the data.
- Send the data to a place of your choice (for example, a Postgres database or a file).

<figure><img src=".gitbook/assets/intro/supported-chains.png" width="100%" /></figure>

**You can use Substreams packages to define which specific data you want to extract from the blockchain**. For example, consider that you want to retrieve data from the Uniswap v3 smart contract. You can simply use the [Uniswap v3 Substreams Package](https://substreams.dev/streamingfast/uniswap-v3/v0.2.7) and send that data wherever you want!

## Consume Substreams

There are many ready-to-use Substreams packages, so you can simply consume them. Use the **([Substreams.dev Registry](https://substreams.dev)) to explore packages**.

Once you find a package that fits your needs, you only have choose **how you want to consume the data**. Send the data to a SQL database, configure a webhook or stream directly from your application!

<figure><img src=".gitbook/assets/intro/consume-flow.png" width="100%" /></figure>

## Develop Substreams

If you can't find a Substreams package that retrieves exactly the data you need, **you can develop your own Substreams**.

You can write your own Rust function to extract data from the blockchain:

```rust
fn get_usdt_transaction(block: eth::Block) -> Result<Vec<Transaction>, substreams:error:Error> {
    let my_transactions = block.transactions().
        .filter(|transaction| transaction.to == USDT_CONTRACT_ADDRESS)
        .map(|transaction| MyTransaction(transaction.hash, transaction.from, transaction.to))
        .collect();
    Ok(my_transactions)
}
```

<figure><img src=".gitbook/assets/intro/develop-flow.png" width="100%" /></figure>

## How Does It Work?

The following video covers how Substreams works in less than 2 minutes:

{% embed url="https://www.youtube.com/watch?v=gVqGCqKVM08" %}
Get an overview of Substreams
{% endembed %}