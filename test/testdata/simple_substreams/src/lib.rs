mod pb;
use substreams::{
    errors,
    store::{ProtoStoreSet, StoreAddInt64, StoreSet, Deltas, I64Delta, StoreAddBigInt, BigIntDelta, BigIntStoreGet}, scalar::BigInt, log,
};

use crate::pb::test;

#[substreams::handlers::map]
fn test_map(blk: test::Block) -> Result<test::MapResult, errors::Error> {
    let out = test::MapResult {
        block_number: blk.number,
        block_hash: blk.id,
    };
    Ok(out)
}

#[substreams::handlers::store]
fn test_store_proto(map_result: test::MapResult, s: ProtoStoreSet<test::MapResult>) {
    let key = format!("result:{}", map_result.block_hash);
    s.set(1, key, &map_result)
}

#[substreams::handlers::store]
fn test_store_add_int64(_blk: test::Block, s: StoreAddInt64) {
    s.add(1, "a.key", 1)
}

//#[substreams::handlers::map]
//fn assert_test_store_add_int64(d:Deltas<I64Delta>, s: StoreGet<Int64>) -> Result<bool, errors::Error> {
//    Ok(true)
//}

const TO_BE_ADDED:u64 = 1;
const TO_BE_SUBTRACTED:i64 = -1;
substracted

fn expected_operation(block_num: u64) -> substreams::pb::substreams::store_delta::Operation {
    let mut op =  substreams::pb::substreams::store_delta::Operation::Update;
    if block_num == 1 {
        op =  substreams::pb::substreams::store_delta::Operation::Create;
    }
    op
}

#[substreams::handlers::store]
fn test_store_add_bigint(_blk: test::Block, s: StoreAddBigInt) {
    s.add(1, "a.key.pos", BigInt::from(TO_BE_ADDED));
    s.add(1, "a.key.neg", BigInt::from(TO_BE_SUBTRACTED));
}

#[substreams::handlers::map]
fn assert_test_store_add_bigint(block: test::Block, deltas:Deltas<BigIntDelta>, s: BigIntStoreGet) -> Result<bool, errors::Error> {
    assert_eq!(BigInt::from(block.number*TO_BE_ADDED), s.get_last("a.key.pos").unwrap());
    assert_eq!(BigInt::from(block.number as i64*TO_BE_SUBTRACTED), s.get_last("a.key.neg").unwrap());

    let expected_operation = expected_operation(block.number);

    assert_eq!(expected_operation, deltas.deltas[0].operation);
    assert_eq!(expected_operation, deltas.deltas[1].operation);

    assert_eq!(BigInt::from(block.number*TO_BE_ADDED), deltas.deltas[0].new_value);
    assert_eq!(BigInt::from(block.number as i64*TO_BE_SUBTRACTED), deltas.deltas[1].new_value);

    Ok(true)
}

//todo: test get_first
//todo: test get_last
//todo: test delete_prefix