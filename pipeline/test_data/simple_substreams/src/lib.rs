use substreams;
use substreams::{
    errors,
    store::{ProtoStoreSet, StoreAddInt64, StoreSet},
};

use crate::pb::test;

mod pb;

#[substreams::handlers::map]
fn map_test(blk: test::Block) -> Result<test::MapResult, errors::Error> {
    let out = test::MapResult {
        block_number: blk.number,
        block_hash: blk.id,
    };
    Ok(out)
}

#[substreams::handlers::store]
fn store_map_result(map_result: test::MapResult, s: ProtoStoreSet<test::MapResult>) {
    let key = format!("result:{}", map_result.block_hash);
    s.set(1, key, &map_result)
}

#[substreams::handlers::store]
fn store_add_int64(_map_result: test::MapResult, s: StoreAddInt64) {
    s.add(1, "sum.key", 1)
}
