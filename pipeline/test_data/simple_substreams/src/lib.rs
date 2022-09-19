use substreams::{log, store, pb, errors, proto};
use substreams;

#[substreams::handlers::map]
fn map_test(blk: pb::test::Block) -> Result<pb::test::MapResult, errors::Error> {
    let out = pb::test::MapResult{ block_number: blk.number, block_hash: blk.id };
    Ok(out)
}

#[substreams::handlers::store]
fn store_map_result(map_result: pb::test::MapResult, s: store::StoreSet) {
    let key = format!("result:{}", map_result.block_hash);
    s.set(1, key, &proto::encode(&map_result).unwrap())
}

#[substreams::handlers::store]
fn store_add_int64(map_result: pb::test::MapResult, s: store::StoreAddInt64) {
    s.add(1, "sum", 1)
}