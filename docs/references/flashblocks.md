# Flashblocks Support (beta)

{% hint style="info" %}
**Flashblocks support temporarily disabled**

As of May 13th, Flashblocks support has been disabled while we switch to op-reth instrumented nodes. It will be restored in the coming weeks.
{% endhint %}

New support for "Flashblocks" is now available as a *beta feature* on Base Mainnet. For more details about Base Flashblocks, see the [Base documentation](https://docs.base.org/base-chain/flashblocks/apps).

{% hint style="warning" %}
**Disclaimers**

* Only the `Base Mainnet` endpoint (https://base-mainnet.streamingfast.io) supports Flashblocks at this time.
* It is normal to sometimes skip some flash blocks indexes. In Substreams, the data from missing flash blocks will always be bundled in the next block so you won't miss any data.
* Substreams only sends an "undo signal" in case of a reorg, or if the sent partial blocks are being discarded (replaced by a different block). It will not send "undo signals" between each partial block with the same block height.
{% endhint %}

## Description

Flashblocks are partial blocks that are not fully confirmed yet. 
They are emitted every 200ms and contain a fraction of the transactions that will be in the final block.
Consuming them allows you to get access to transaction data as soon as it's sequenced, rather than waiting for full block confirmation.
Transactions can be processed incrementally, making your applications more responsive or predictions more accurate.

## Flashblocks in Substreams

### Partial Blocks

* In Substreams, Flashblocks are called **partial blocks**, as a generalization of the concept, even though Flashblocks are the only supported implementation yet.
* To benefit from partial blocks:
  - You need a recent version of Substreams CLI or library (> v1.17.9).
  - Your Substreams modules should avoid doing "block-level aggregations" and should only work on what is inside the "transactionTraces" -- to avoid non-deterministic output.
  - Your Substreams sink implementation should only take decisions on the block hash if it receives a full block or the "last_partial_block"

Here's how it works:

1. The "sequencer" emits a flashblock every 200ms (so a maximum of 10 per block height)
1. The instrumented Base node reader sends the increasing versions of the same block to the Substreams engine and eventually, the full block.
1. To keep up with the chain, it may skip a few emissions of partial blocks, but will never send the transactions out-of-order.
1. The Substreams engine will remember what was processed for each active Substreams and only process the new transactions since the last execution.
1. It sends the data inside [BlockScopedData](https://buf.build/streamingfast/substreams/docs/main:sf.substreams.rpc.v2#sf.substreams.rpc.v2.BlockScopedData) for each part of the full block as it gets it from the partial blocks, with `is_partial=true`, with `partial_index` and `is_last_partial` populated.
1. If there is a reorg, an UNDO signal is sent, followed by the correct full blocks for the new chain segment, until we are up to HEAD again and start receiving more partial blocks..

### Changes to Protobuf models

* Partial blocks are sent as regular `[BlockScopedData](https://buf.build/streamingfast/substreams/docs/main:sf.substreams.rpc.v2#sf.substreams.rpc.v2.BlockScopedData)`, with `is_partial=true`. The ordinal of that partial is set in `partial_index` and the last partial will always have `is_last_partial=true`


  ```proto
  message BlockScopedData {
    ...
    bool is_partial = 13;
	  // Only present if is_partial==true
	  optional uint32 partial_index = 14;
	  // Only present if is_partial==true
	  // true if this is the last partial of a given block, this will be the correct hash of the block (unless there are reorgs)
	  optional bool is_last_partial = 15;
  }
  ```

* The [`sf.substreams.rpc.v2.Request`](https://buf.build/streamingfast/substreams/docs/main:sf.substreams.rpc.v2#sf.substreams.rpc.v2.Request) and [`sf.substreams.rpc.v3.Request`](https://buf.build/streamingfast/substreams/docs/main:sf.substreams.rpc.v3#sf.substreams.rpc.v3.Request) now contain this parameter:
  
  ```proto
  // If true, blocks close to head will be sent in "partials" as soon as we get them.
	// This means that you will get different versions of the same block number, each an incomplete increment
	// Other blocks will be sent completely (older blocks, or blocks for which the provider did not get a partial in time)
	bool partial_blocks = 16;
  ```

## Developing for partial blocks

When writing a substreams that will run on partial blocks, remember that your modules will run multiple times on small increments of the same block.
This means that any type of aggregation in a mapper will be incorrect. Only process data inside the block as if it were a stream of transactions.
Also, never use the block hash in your modules, as it changes between the versions of a partial block.

### Example of workflow

For the hypothetical scenario where:
* block #122 already exists at the time of the substreams connection
* a block #123 is being emitted as partial blocks
* each partial block contains exactly 10 new transactions (to simplify the example)
* the Substreams engine receives only the blocks with index 2, 4, 7 (some may be skipped to keep up with the chain HEAD)
* finally, it receives the full block #123

The module will be executed on full block #122 (with transactions 0-100).

Then, the module will be executed 4 times with partial data:
  1. with transactions 0-20
  1. with transactions 20-40
  1. with transactions 40-70
  1. with transaction 70-100 (when it gets the full block)
  
The user will receive 5 `BlockScopedData` messages:
  1. The full block #122, with `Clock(num=122, ID=...)` and `isPartial=false`
  1. The result of execution of trx 0-20, with `Clock(num=123, ID=0x123aaaaaa)`, `isPartial=true`, `partialIndex=2`, `isLastPartial=false`
  1. The result of execution of trx 20-40, with `Clock(num=123, ID=0x123bbbbbbb)`, `isPartial=true`, `partialIndex=4`, `isLastPartial=false`
  1. The result of execution of trx 40-70, with `Clock(num=123, ID=0x123ccccccc)`, `isPartial=true`, `partialIndex=7`, `isLastPartial=false`
  1. The result of execution of trx 70-100, with `Clock(num=123, ID=0x123ddddddd)`, `isPartial=true`, `partialIndex=10`, `isLastPartial=true`

## Consuming partial blocks

### A simple test, from terminal, with `substreams run`

1. Get the latest release of Substreams: https://github.com/streamingfast/substreams/releases/tag/v1.17.9

1. To test with a common module, using `jq` to quickly see what is going on (you need [jq](https://jqlang.org/)):

`substreams run -e https://base-mainnet.streamingfast.io ethereum_common all_events -s -1 --partial-blocks -o json`

You will get responses like this for partial blocks:
```
{
  "@module": "all_events",
  "@block": 41764712,
  "@partial_index": 9,
  "@is_last_partial": false,
  "@type": "sf.substreams.ethereum.v1.Events",
  "@data": {
    (...)
  }
}
```

and sometimes a full block like this one:

```
{
  "@module": "all_events",
  "@block": 41764713,
  "@type": "sf.substreams.ethereum.v1.Events",
  "@data": {
    (...)
  }
}
```

1. To see how it performs with a clock, you can use, as always, the -o clock with something like this:

`substreams run -e https://base-mainnet.streamingfast.io https://github.com/graphprotocol/graph-node/raw/refs/heads/master/substreams/substreams-head-tracker/substreams-head-tracker-v1.0.0.spkg -s -1 -o clock --partial-blocks`

This will print lines like this:
```
----------- BLOCK #41,764,571 (45e2e160e8121e7a9cb2db8c3b10d155cdad73ee5385fc6113c04c8af920411b) age=2.09382s ---------------
----------- PARTIAL BLOCK (idx=10) #41,764,572 (910e532120abc7fbed609f74d23bde300a9b3b4faf46bbf405469c6a63e5aac3) age=119.132ms ---------------
----------- PARTIAL BLOCK (last) #41,764,572 (e53e57ed50fc7a0136d4e092fc1cb11fd02698e16749847f180a126f0397be4e) age=439.173ms ---------------
----------- PARTIAL BLOCK (idx=1) #41,764,573 (198f9754cfc5eff733429ca79421a2039fb6ef76360fa769475630b361290d19) age=-1.499929s ---------------
----------- PARTIAL BLOCK (idx=2) #41,764,573 (52c4605a5bb394ac3436a0d371861c8efaa6ffe912a7ed8e4c80b69b90c241ae) age=-1.4895s ---------------
----------- PARTIAL BLOCK (idx=3) #41,764,573 (a2e2a2a0a67072f05ae6c3c8a41f442daaac6d3685e8aa0070f685eb131cffbc) age=-1.359628s ---------------
----------- PARTIAL BLOCK (idx=4) #41,764,573 (cfd5b34d92ba14bf4901c26541180e74a6057d8174e8b85bcb0ca5628dc441c2) age=-1.201712s ---------------
----------- PARTIAL BLOCK (idx=5) #41,764,573 (752d4b6578bd3bd661713c1b45dd9b545379a5185ff92366ebf7879f5bf43b5d) age=-1.009816s ---------------
----------- PARTIAL BLOCK (idx=6) #41,764,573 (b6e9dfab82bfeb7eb50388d706f6bf01f94fb4dc171b09e56875ff2713592261) age=-756.23ms ---------------
----------- PARTIAL BLOCK (idx=7) #41,764,573 (53eaa9eb18ac5b7d8d49d4c3fa9cebc5fd1c9788e74831d41c5cb8edebf4da14) age=-621.185ms ---------------
----------- PARTIAL BLOCK (idx=8) #41,764,573 (cd2d4b9c0051966db0c8887002f07c3b22c1e3ce04b638fd4049f1699092c98a) age=-457.616ms ---------------
----------- PARTIAL BLOCK (idx=9) #41,764,573 (5b3288a1712e8d6126bbf131ab4608c5d33a57705388ca7785b9c639fbbdff69) age=-252.544ms ---------------
----------- PARTIAL BLOCK (idx=10) #41,764,573 (c14d1553dbc40d99505a51bf1fe9618b0bf884261f54512867dbd24a2346fb7e) age=-129.763ms ---------------
----------- PARTIAL BLOCK (last) #41,764,573 (ae919437884c5f08a1730c2ffe0e390f3d3fd4c2a16258540db12dd12cd58bbc) age=457.636ms ---------------
----------- PARTIAL BLOCK (idx=1) #41,764,574 (da8c1003dc819ff776ddbfa0f44759f828c30ed853530505d7b1c3178df6ba4c) age=-1.541983s ---------------
----------- PARTIAL BLOCK (idx=2) #41,764,574 (6011e585fbbffedce1bb3ee448a7613e40fea20c78d12011af87dae6fb78e382) age=-1.541895s ---------------
```

{% hint style="note" %}
See the "negative age", that's because at partial block with idx=5, the proposed block timestamp is still 2 seconds in the future.
{% endhint %}

### A more useful example, with the Substreams Webhook Sink:

1. Get the latest release of Substreams: https://github.com/streamingfast/substreams/releases
1. Run this command:

`substreams sink webhook --partial-blocks -e https://base-mainnet.streamingfast.io http://webhook.example.com path-to-your.spkg  -s -1`

Enjoy!

### More sinks

Flashblock support is not implemented in other sinks. For example, we believe that it would be a bad idea to implement in the SQL sink, because it would cause too many "undo" operations.
If you are using our [Golang Substreams Sink SDK](https://github.com/streamingfast/substreams/blob/develop/sink/README.md#substreams-sink), you can simply:
  - Bump to the latest version of substreams in your go.mod (1.17.9 and above)
  - Define your sink flags with `sink.FlagPartialBlocks` under `FlagIncludeOptional()`
  - Optionally, add some logic to handle the "IsPartial", "PartialIndex" and "IsLastPartial" attributes in function `HandleBlockScopedData(...)` when creating the sinker.
