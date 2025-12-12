---
description: Flashblocks support (alpha feature)
---

New support for "Flashblocks" is now available for alpha testing on Base Mainnet.

ref: https://docs.base.org/base-chain/flashblocks/apps

## Disclaimers

* The only endpoint supporting Flashblocks is `https://base-mainnet-flash.streamingfast.io:443`.
* That endpoint is not guaranteed to be stable or available at all times.
* The protocol might still change in the next few weeks, as we gather feedback on usage.
* Flashblocks coming from Firehose or Substreams endpoint should not be considered final data. Only write to database data coming from full blocks: there is no "undo" mechanism for Flashblocks.

## Description

* Flashblocks are partial blocks that are not fully confirmed yet. 
* They are emitted every 200ms and contain a fraction of the transactions that will be in the final block.

### Flashblocks in Substreams

#### Partial Blocks

* In Substreams, Flashblocks are called **partial blocks**, as a generalization of the concept, even though Flashblocks are the only supported implementation yet.
* To benefit from partial blocks: 
  1. you need the latest version of substreams CLI or library.
  2. your substreams modules should avoid doing "block-level aggregations" and should only work on what is inside the "transactionTraces"

Here's how it works:

1. The "sequencer" emits a flashblock every 200ms (so a maximum of 10 per block height)
2. The instrumented Base node reader sends the increasing versions of the same block to the Substreams engine and eventually, the full block.
3. To keep up with the chain, it may skip a few emissions of partial blocks, but will never send the transactions out-of-order.
4. The Substreams engine will remember what was processed for each active Substreams and only process the new transactions since the last execution.
5. It sends the PartialData for each part of the full block as it gets it from the flash blocks
6. If there is a reorg, new and undo signals are sent for the full blocks, but there is no consideration for the sent partial blocks. The user must always consider that the partial blocks data may become invalid.
7. For this reason, there is no "cursor" sent with the partial blocks data.

#### Changes to Protobuf models

* Partial blocks are not sent as `BlockScopedData`, but as `PartialBlockData`, which is new possible type for `Response.message`

```proto
// PartialBlockData represents partial block data from "flashblocks".
// Flashblocks are partial, unconfirmed blocks emitted every ~200ms containing a fraction
// of the transactions that will be in the final block.
//
// Important characteristics:
// * Partial blocks are NOT final and may become invalid if there's a reorg
// * No cursor is sent with partial blocks - they cannot be used for resumption
// * The same block number will have multiple partial blocks with increasing partial_index
// * Eventually a full BlockScopedData will be sent for the same block (when include_partial_blocks is true)
// * Only the full block data is used to update stores, ensuring consistency
// * Transactions are sent incrementally and never out-of-order within a block
// * Some partial block emissions may be skipped to keep up with chain head
//
// Enable by setting include_partial_blocks or partial_blocks_only in the Request.
// See: https://docs.base.org/base-chain/flashblocks/apps
message PartialBlockData {
  // The output of your module execution on the partial block's transactions
  MapModuleOutput output = 1;

  // Clock contains the block number and ID for this partial block.
  // Note: Each partial block has a different, temporary block ID. Only the last partial block's
  // ID will match the final confirmed block's ID when the full block is produced.
  sf.substreams.v1.Clock clock = 2;

  // partial_index indicates which partial block this is (e.g., 1, 2, 3...).
  // Increases sequentially for each emission of the same block number.
  // The engine may skip some indices to keep up with chain HEAD, but transactions
  // are always sent in order without duplication.
  uint32 partial_index = 3;
}
```

* The `sf.substreams.rpc.v2.Blocks/Request`  and `sf.substreams.rpc.v3.Blocks/Request` now contain these parameters:

``` proto
// If true, partial blocks (flashblocks) are sent in addition to full blocks.
// Partial blocks are unconfirmed, incremental block data emitted every ~200ms.
// When enabled, you'll receive multiple PartialBlockData messages for each block,
// followed by the full BlockScopedData for that block.
// Note: Partial blocks have no cursor and may become invalid on reorg.
bool include_partial_blocks = 15;

// If true, only partial blocks (flashblocks) are sent - no BlockScopedData or cursor.
// This is useful for real-time monitoring where you don't need confirmed data.
// This value supersedes include_partial_blocks.
// Note: Without cursors, you cannot resume from a specific point.
bool partial_blocks_only = 16;
```

#### Example

For the hypothetical scenario where 
* a block #123 is being emitted as partial blocks
* each partial block contains exactly 10 new transactions (to simplify the example)
* the Substreams engine receives only the blocks with index 2, 4, 7 (some may be skipped to keep up with the chain HEAD)
* finally, it receives the full block #123

The module will be executed 4 times with partial data:
  1. with transactions 0-20
  2. with transactions 20-40
  3. with transactions 40-70
  4. with transaction 70-100 (when it gets the full block)
  
Then, the module will be executed again with the full block data. This is the data that will be used to apply changes to the stores, to ensure consistency before we execute the next blocks.
      
The user will receive:

1. The result of execution of trx 0-20, within PartialBlockData with Clock(num=123, ID=0xaaaaaaaaa) and PartialIndex=2
2. The result of execution of trx 20-40, within PartialBlockData with Clock(num=123, ID=0xbbbbbbbbbb) and PartialIndex=4
3. The result of execution of trx 40-70, within PartialBlockData with Clock(num=123, ID=0xcccccccccc) and PartialIndex=7
4. The result of execution of trx 70-100, within PartialBlockData with Clock(num=123, ID=0xdddddddddd) and PartialIndex=10

If the user requested `include_partial_blocks` (and NOT `partial_blocks_only`), they will also receive:

5. The result of execution of trx 0-100, within BlockData with Clock (num=123, ID=0xdddddddddd) <- Note that the ID here is the same


## Using Flashblocks

### A simple test, from terminal, with `substreams run`

1. Get the latest release of Substreams: https://github.com/streamingfast/substreams/releases/tag/v1.17.8

2. To test with a common module, using `jq` to quickly see what is going on (you need [jq](https://jqlang.org/)):

`substreams run -e https://base-mainnet-flash.streamingfast.io ethereum_common all_events -s -1 --include-partial-blocks -o jsonl | jq -r '"Block: #\(.["@block"]) Partial: \(.["@partial_index"]) Event count:\(.["@data"].events|length)"'`

This will print lines like this:

```
Block: #39306107 Partial: 5 Event count:531
Block: #39306107 Partial: 7 Event count:152
Block: #39306107 Partial: 9 Event count:182
Block: #39306107 Partial: 10 Event count:127
Block: #39306107 Partial: null Event count:992
Block: #39306108 Partial: 3 Event count:899
Block: #39306108 Partial: 5 Event count:197
Block: #39306108 Partial: 7 Event count:107
Block: #39306108 Partial: 9 Event count:121
Block: #39306108 Partial: 10 Event count:77
Block: #39306108 Partial: null Event count:1401
```

When you see "partial: null" it means that it is the actual full block.

3. To see how it performs with a clock, you can use, as always, the -o clock with something like this:

`substreams run -e https://base-mainnet-flash.streamingfast.io https://github.com/graphprotocol/graph-node/raw/refs/heads/master/substreams/substreams-head-tracker/substreams-head-tracker-v1.0.0.spkg -s -1 -o clock --include-partial-blocks`

This will print lines like this:
```
----------- PARTIAL BLOCK #39,306,388 (idx=7) (bcbcc30d0d6f4d5581519f0da90fe8b61ab8ae4e38193c1714285bee2be5cb98) age=-484.276ms ---------------
----------- PARTIAL BLOCK #39,306,388 (idx=9) (0399532628e8b7585c53a58b3bce5d6432fc76b3aee4730292ab4cef8c675ccf) age=-181.9ms ---------------
----------- PARTIAL BLOCK #39,306,388 (idx=10) (c7871f81180ad6565ae6f9c4d244b2dcac05b2348aff54bd591a65d6c945107e) age=1.138322s ---------------
----------- BLOCK #39,306,388 (c7871f81180ad6565ae6f9c4d244b2dcac05b2348aff54bd591a65d6c945107e) age=1.138495s ---------------
```
See the "negative age", that's because at partial block with idx=5, the proposed block timestamp is still 2 seconds in the future.

### A more useful example, with the Substreams Webhook Sink:

1. Get the latest release of Substreams: https://github.com/streamingfast/substreams/releases/tag/v1.17.8
2. Run this command:

`substreams sink webhook --partial-blocks-only -e https://base-mainnet-flash.streamingfast.io http://webhook.example.com path-to-your.spkg  -s -1`

Enjoy!

### More sinks

Flashblock support is not implemented in other sinks. For example, we believe that it would be a bad idea to implement in the SQL sink, because it would cause too many "undo" operations.
If you have your own sink implementation that uses github.com/streamingfast/substreams/sink, you can simply:
  - Bump to the latest version of substreams in your go.mod (1.17.8 and above)
  - Define your sink flags with `sink.FlagIncludePartialBlocks` and/or `sink.FlagPartialBlocksOnly` under `FlagIncludeOptional()`
  - Implement the function `HandlePartialBlockData(...)` and pass it to `NewSinkerFullHandlersWithPartial(...)` when creating the sinker.
  - Enjoy!
