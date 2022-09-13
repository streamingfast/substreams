use substreams::{log, store, pb, errors};
use substreams;

#[substreams::handlers::map]
fn map_test(blk: pb::test::Block) -> Result<pb::test::MapResult, errors::Error> {
    let out = pb::test::MapResult{ block_number: 0, block_hash: "".to_string() };
    Ok(out)
}
/// Store the total balance of NFT tokens for the specific TRACKED_CONTRACT by holder
#[substreams::handlers::store]
fn store_transfers(pb::test::MapResult, s: store::StoreAddInt64) {
}