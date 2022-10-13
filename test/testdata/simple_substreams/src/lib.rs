use substreams::store::{
    DeltaBigInt, DeltaI64, StoreGetBigInt, StoreGetI64, StoreSetBigInt, StoreSetI64, StoreSetProto,
};
use substreams::{
    errors, log,
    scalar::BigInt,
    store::{Deltas, StoreAddBigInt, StoreAddInt64, StoreGet, StoreSet},
};

use crate::pb::test;

mod pb;

#[substreams::handlers::map]
fn test_map(blk: test::Block) -> Result<test::MapResult, errors::Error> {
    let out = test::MapResult {
        block_number: blk.number,
        block_hash: blk.id,
    };
    Ok(out)
}

#[substreams::handlers::store]
fn test_store_proto(map_result: test::MapResult, s: StoreSetProto<test::MapResult>) {
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

const TO_SET: i64 = 100;
const TO_ADD: i64 = 1;
const TO_SUBTRACT: i64 = -1;

fn expected_operation(block_num: u64) -> substreams::pb::substreams::store_delta::Operation {
    let mut op = substreams::pb::substreams::store_delta::Operation::Update;
    if block_num == 1 {
        op = substreams::pb::substreams::store_delta::Operation::Create;
    }
    op
}

#[substreams::handlers::store]
fn test_store_add_bigint(_blk: test::Block, s: StoreAddBigInt) {
    s.add(1, "a.key.neg", BigInt::from(TO_SUBTRACT));
}

#[substreams::handlers::map]
fn assert_test_store_add_bigint(
    block: test::Block,
    deltas: Deltas<DeltaBigInt>,
    s: StoreGetBigInt,
) -> Result<bool, errors::Error> {
    assert_eq!(
        BigInt::from(block.number as i64 * TO_ADD),
        s.get_last("a.key.pos").unwrap()
    );
    assert_eq!(
        BigInt::from(block.number as i64 * TO_SUBTRACT),
        s.get_last("a.key.neg").unwrap()
    );

    let expected_operation = expected_operation(block.number);

    assert_eq!(expected_operation, deltas.deltas[0].operation);
    assert_eq!(expected_operation, deltas.deltas[1].operation);

    assert_eq!(
        BigInt::from(block.number as i64 * TO_ADD),
        deltas.deltas[0].new_value
    );
    assert_eq!(
        BigInt::from(block.number as i64 * TO_SUBTRACT),
        deltas.deltas[1].new_value
    );

    Ok(true)
}

#[substreams::handlers::store]
fn test_store_delete(block: test::Block, s: StoreSetI64) {
    let to_set_key = format!("key:{}", block.number);
    s.set(1, to_set_key, &TO_SET);

    if block.number > 1 {
        let previous_block_num = block.number - 1;
        let to_delete_key = format!("key:{}", previous_block_num);
        s.delete_prefix(9, &to_delete_key)
    }
}

#[substreams::handlers::map]
fn assert_test_store_delete(
    block: test::Block,
    deltas: Deltas<DeltaI64>,
    s: StoreGetI64,
) -> Result<bool, errors::Error> {
    let to_read_key = format!("key:{}", block.number);
    assert_eq!(TO_SET, s.get_last(to_read_key).unwrap());

    if block.number > 1 {
        let previous_block_num = block.number - 1;
        let deleted_key = format!("key:{}", previous_block_num);
        assert_eq!(None, s.get_last(deleted_key))
    }

    //todo test deltas

    Ok(true)
}

//todo: test get_first
//todo: test get_last
//todo: test delete_prefix
