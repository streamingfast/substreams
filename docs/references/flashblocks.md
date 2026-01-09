# Flashblocks Support (alpha)

New support for "Flashblocks" is now available for alpha testing on Base Mainnet. For more details about Base Flashblocks, see the [Base documentation](https://docs.base.org/base-chain/flashblocks/apps).

{% hint style="warning" %}
**Disclaimers**

* The only endpoint currently supporting Flashblocks is `https://base-mainnet-flash.streamingfast.io:443`.
* That endpoint is not guaranteed to be stable or available at all times (alpha testing).
* The protocol might still change in the next few weeks, as we gather feedback on usage.
* It is normal to receive only "some" partial blocks indexes. In Substreams, the data from missing ones will always be bundled in the next PartialBlockData.
* Substreams only sends an "undo signal" in case of a reorg, or if the sent partial blocks are being discarded and the new block does not correspond to the partial blocks sent. It will not send "undo signals" between each partial block with the same block height and different block hash.
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
  - You need the latest version of Substreams CLI or library (v1.17.9).
  - Your Substreams modules should avoid doing "block-level aggregations" and should only work on what is inside the "transactionTraces"
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

`substreams run -e https://base-mainnet-flash.streamingfast.io ethereum_common all_events -s -5 --partial-blocks -o jsonl | jq -r '"Block: #\(.["@block"]) Partial: \(.["@partial_index"]) (last:\(.["@is_last_partial"])) Event count:\(.["@data"].events|length)"'`

{% hint style="note" %}
The `jq` part is optional, only used here to show a quick summary of the content. Without it, you would receive the full JSON objects with the ethereum events..
{% endhint %}

This will print lines like this:

```
Block: #40591619 Partial: null (last:null) Event count:1403
Block: #40591620 Partial: null (last:null) Event count:975
Block: #40591621 Partial: null (last:null) Event count:1366
Block: #40591622 Partial: null (last:null) Event count:1545
Block: #40591623 Partial: null (last:null) Event count:789
Block: #40591624 Partial: 5 (last:false) Event count:533
Block: #40591624 Partial: 7 (last:false) Event count:78
Block: #40591624 Partial: 9 (last:false) Event count:711
Block: #40591624 Partial: 10 (last:true) Event count:246
Block: #40591625 Partial: 5 (last:false) Event count:935
Block: #40591625 Partial: 7 (last:false) Event count:108
Block: #40591625 Partial: 9 (last:false) Event count:148
Block: #40591625 Partial: 10 (last:true) Event count:0
```

When you see "partial: null" and "last:null", it means that it is the actual full block.

1. To see how it performs with a clock, you can use, as always, the -o clock with something like this:

`substreams run -e https://base-mainnet-flash.streamingfast.io https://github.com/graphprotocol/graph-node/raw/refs/heads/master/substreams/substreams-head-tracker/substreams-head-tracker-v1.0.0.spkg -s -1 -o clock --partial-blocks`

This will print lines like this:
```
----------- BLOCK #40,591,725 (cdfc46f244004b890fabcb7e92234e12a3a146473f1001797c7e8bbd86009282) age=1.775644s ---------------
----------- PARTIAL BLOCK (idx=7) #40,591,726 (366bef1c98d1e2f1f548e67a984ccc607450b599356036164b318de1d096a53d) age=-205.354ms ---------------
----------- PARTIAL BLOCK (idx=9) #40,591,726 (f060709ceac09c44b1af22a8889b9f7988fb59f6efbd70dda6cd5cc56e4bcec5) age=58.385ms ---------------
----------- PARTIAL BLOCK (last) #40,591,726 (514c078213650a19c5e262d836217c2c85e79a5422037ee3a30de0456965ec84) age=871.572ms ---------------
----------- PARTIAL BLOCK (idx=5) #40,591,727 (0f10c34677af8101e9485e19f709ea749b0c7f386c98eb9f20047ad2febee121) age=-581.566ms ---------------
----------- PARTIAL BLOCK (idx=7) #40,591,727 (6fae223728d9ea46ca12e6e30fcd025158a9da631795df27d4bfbd352ba7174b) age=-545.254ms ---------------
----------- PARTIAL BLOCK (idx=9) #40,591,727 (117195eeae67615c01cc4def371e8783b182b73172337b6c31b617bd6b1f93bd) age=121.267ms ---------------
----------- PARTIAL BLOCK (last) #40,591,727 (5c284f1bf94363dc71c60d9fb7a4c12a828c2bd258fdf7fe6ea5d9628b9bb20a) age=1.301598s ---------------
```
{% hint style="note" %}
See the "negative age", that's because at partial block with idx=5, the proposed block timestamp is still 2 seconds in the future.
{% endhint %}

### A more useful example, with the Substreams Webhook Sink:

1. Get the latest release of Substreams: https://github.com/streamingfast/substreams/releases/tag/v1.17.9
1. Run this command:

`substreams sink webhook --partial-blocks -e https://base-mainnet-flash.streamingfast.io http://webhook.example.com path-to-your.spkg  -s -1`

Enjoy!

### More sinks

Flashblock support is not implemented in other sinks. For example, we believe that it would be a bad idea to implement in the SQL sink, because it would cause too many "undo" operations.
If you are using our [Golang Substreams Sink SDK](https://github.com/streamingfast/substreams/blob/develop/sink/README.md#substreams-sink), you can simply:
  - Bump to the latest version of substreams in your go.mod (1.17.9 and above)
  - Define your sink flags with `sink.FlagPartialBlocks` under `FlagIncludeOptional()`
  - Optionally, add some logic to handle the "IsPartial", "PartialIndex" and "IsLastPartial" attributes in function `HandleBlockScopedData(...)` when creating the sinker.
