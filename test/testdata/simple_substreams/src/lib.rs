mod pb;

use crate::pb::test;
use std::fmt::{Debug, Display};

use substreams::{
    errors,
    scalar::BigInt,
    store::{
        DeltaBigInt, DeltaI64, Deltas, StoreAdd, StoreAddBigInt, StoreAddInt64, StoreDelete,
        StoreGet, StoreGetBigInt, StoreGetI64, StoreNew, StoreSet, StoreSetI64, StoreSetProto,
    },
};

const TO_SET: i64 = 100;
const TO_ADD: i64 = 1;
const TO_SUBTRACT: i64 = -1;

#[substreams::handlers::map]
fn test_map(blk: test::Block) -> Result<test::MapResult, errors::Error> {
    let out = test::MapResult {
        block_number: blk.number,
        block_hash: blk.id,
    };
    Ok(out)
}

#[substreams::handlers::store]
fn test_store_add_int64(_blk: test::Block, s: StoreAddInt64) {
    s.add(1, "a.key", 1)
}

#[substreams::handlers::store]
fn test_store_proto(map_result: test::MapResult, s: StoreSetProto<test::MapResult>) {
    let key = format!("result:{}", map_result.block_hash);
    s.set(1, key, &map_result)
}

#[substreams::handlers::store]
fn test_store_add_bigint(_blk: test::Block, s: StoreAddBigInt) {
    s.add(1, "a.key.pos", BigInt::from(TO_ADD));
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
fn test_store_delete_prefix(block: test::Block, s: StoreSetI64) {
    let to_set_key = format!("key:{}", block.number);
    s.set(block.number, to_set_key, &TO_SET);

    if block.number > 1 {
        let previous_block_num = block.number - 1;
        let to_delete_key = format!("key:{}", previous_block_num);
        s.delete_prefix(block.number as i64, &to_delete_key)
    }
}

#[substreams::handlers::map]
fn assert_test_store_delete_prefix(
    block: test::Block,
    s: StoreGetI64,
) -> Result<bool, errors::Error> {
    let to_read_key = format!("key:{}", block.number);
    assert_eq!(TO_SET, s.get_last(to_read_key).unwrap());

    if block.number > 1 {
        let previous_block_num = block.number - 1;
        let deleted_key = format!("key:{}", previous_block_num);
        assert_eq!(None, s.get_last(deleted_key))
    }

    Ok(true)
}

// -------------------- StoreAddI64 -------------------- //
#[substreams::handlers::store]
fn setup_test_store_add_i64(block: test::Block, s: StoreAddInt64) {
    s.add(block.number, "a.key", i64::MAX);
    s.add(block.number, "a.key", i64::MIN);
    s.add(block.number, "a.key", 1);
}

#[substreams::handlers::map]
fn assert_test_store_add_i64(block: test::Block, s: StoreGetI64) -> Result<bool, errors::Error> {
    assert(block.number, 0, s.get_last("a.key").unwrap());
    Ok(true)
}

#[substreams::handlers::map]
fn assert_test_store_add_i64_deltas(
    block: test::Block,
    _store: StoreGetI64,
    deltas: Deltas<DeltaI64>,
) -> Result<bool, errors::Error> {
    if deltas.deltas.len() != 3 {
        panic!("expected 3 deltas, got {}", deltas.deltas.len());
    }

    let delta_0 = deltas.deltas.get(0).unwrap();
    assert(block.number, 0, delta_0.old_value);
    assert(block.number, i64::MAX, delta_0.new_value);

    let delta_1 = deltas.deltas.get(1).unwrap();
    assert(block.number, i64::MAX, delta_1.old_value);
    assert(block.number, -1, delta_1.new_value);

    let delta_2 = deltas.deltas.get(2).unwrap();
    assert(block.number, -1, delta_2.old_value);
    assert(block.number, 0, delta_2.new_value);

    Ok(true)
}

// -------------------- StoreSetI64/StoreGetI64 -------------------- //
#[substreams::handlers::store]
fn setup_test_store_set_i64(block: test::Block, store: StoreSetI64) {
    store.set(block.number, "0", &0);
    store.set(block.number, "min", &i64::MIN);
    store.set(block.number, "max", &i64::MAX);
}

#[substreams::handlers::map]
fn assert_test_store_set_i64(block: test::Block, s: StoreGetI64) -> Result<bool, errors::Error> {
    assert(block.number, 0, s.get_last("0").unwrap());
    assert(block.number, i64::MIN, s.get_last("min").unwrap());
    assert(block.number, i64::MAX, s.get_last("max").unwrap());
    Ok(true)
}

#[substreams::handlers::map]
fn assert_test_store_set_i64_deltas(
    block: test::Block,
    _s: StoreGetI64,
    deltas: Deltas<DeltaI64>,
) -> Result<bool, errors::Error> {
    if deltas.deltas.len() != 3 {
        panic!("expected 3 deltas, got {}", deltas.deltas.len());
    }

    let delta_0 = deltas.deltas.get(0).unwrap();
    assert(block.number, 0, delta_0.new_value);
    let delta_1 = deltas.deltas.get(1).unwrap();
    assert(block.number, i64::MIN, delta_1.new_value);
    let delta_2 = deltas.deltas.get(2).unwrap();
    assert(block.number, i64::MAX, delta_2.new_value);

    Ok(true)

}

#[substreams::handlers::store]
fn assert_all_test(
        _assert_test_store_add_bigint: bool,
        _assert_test_store_delete_prefix: bool,
        _assert_test_store_add_i64: bool,
        _assert_test_store_add_i64_deltas: bool,
        _assert_test_store_set_i64: bool,
        _assert_test_store_set_i64_deltas: bool)
{
    //nop!
}

fn assert<T: Debug + Display + PartialEq>(block_number: u64, expected_value: T, actual_value: T) {
    assert_eq!(
        expected_value, actual_value,
        "expected {} got {} at block {}",
        expected_value, actual_value, block_number
    )
}

fn expected_operation(block_num: u64) -> substreams::pb::substreams::store_delta::Operation {
    let mut op = substreams::pb::substreams::store_delta::Operation::Update;
    if block_num == 1 {
        op = substreams::pb::substreams::store_delta::Operation::Create;
    }
    op
}
