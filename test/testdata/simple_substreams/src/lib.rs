mod pb;

use crate::pb::test;
use std::fmt::{Debug, Display};

use crate::test::Block;
use substreams::prelude::{DeltaInt64, StoreDelete};
use substreams::store::{Appender, DeltaArray, DeltaString, StoreAppend, StoreSetString};
use substreams::{
    errors,
    scalar::BigInt,
    store::{
        DeltaBigInt, Deltas, StoreAdd, StoreAddBigInt, StoreAddInt64, StoreGet, StoreGetArray,
        StoreGetBigInt, StoreGetInt64, StoreGetString, StoreNew, StoreSet, StoreSetInt64,
        StoreSetProto,
    },
};

const TO_SET: i64 = 100;
const TO_ADD: i64 = 1;
const TO_SUBTRACT: i64 = -1;

#[substreams::handlers::map]
fn test_map(blk: Block) -> Result<test::MapResult, errors::Error> {
    let out = test::MapResult {
        block_number: blk.number,
        block_hash: blk.id,
    };
    Ok(out)
}

#[substreams::handlers::store]
fn test_store_add_int64(_blk: Block, s: StoreAddInt64) {
    s.add(1, "a.key", 1)
}

#[substreams::handlers::store]
fn test_store_proto(map_result: test::MapResult, s: StoreSetProto<test::MapResult>) {
    let key = format!("result:{}", map_result.block_hash);
    s.set(1, key, &map_result)
}

#[substreams::handlers::store]
fn test_store_add_bigint(_blk: Block, s: StoreAddBigInt) {
    s.add(1, "a.key.pos", BigInt::from(TO_ADD));
    s.add(1, "a.key.neg", BigInt::from(TO_SUBTRACT));
}

#[substreams::handlers::map]
fn assert_test_store_add_bigint(
    block: Block,
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
fn test_store_delete_prefix(block: Block, s: StoreSetInt64) {
    let to_set_key = format!("key:{}", block.number);
    s.set(block.number, to_set_key, &TO_SET);

    if block.number > 1 {
        let previous_block_num = block.number - 1;
        let to_delete_key = format!("key:{}", previous_block_num);
        s.delete_prefix(block.number as i64, &to_delete_key)
    }
}

#[substreams::handlers::map]
fn assert_test_store_delete_prefix(block: Block, s: StoreGetInt64) -> Result<bool, errors::Error> {
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
fn setup_test_store_add_i64(block: Block, s: StoreAddInt64) {
    s.add(block.number, "a.key", i64::MAX);
    s.add(block.number, "a.key", i64::MIN);
    s.add(block.number, "a.key", 1);
}

#[substreams::handlers::map]
fn assert_test_store_add_i64(block: Block, s: StoreGetInt64) -> Result<bool, errors::Error> {
    assert(block.number, 0, s.get_last("a.key").unwrap());
    Ok(true)
}

#[substreams::handlers::map]
fn assert_test_store_add_i64_deltas(
    block: Block,
    _store: StoreGetInt64,
    deltas: Deltas<DeltaInt64>,
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

// -------------------- StoreSetInt64/StoreGetInt64 -------------------- //
#[substreams::handlers::store]
fn setup_test_store_set_i64(block: Block, store: StoreSetInt64) {
    store.set(block.number, "0", &0);
    store.set(block.number, "min", &i64::MIN);
    store.set(block.number, "max", &i64::MAX);
}

#[substreams::handlers::map]
fn assert_test_store_set_i64(block: Block, s: StoreGetInt64) -> Result<bool, errors::Error> {
    assert(block.number, 0, s.get_last("0").unwrap());
    assert(block.number, i64::MIN, s.get_last("min").unwrap());
    assert(block.number, i64::MAX, s.get_last("max").unwrap());
    Ok(true)
}

#[substreams::handlers::map]
fn assert_test_store_set_i64_deltas(
    block: Block,
    _s: StoreGetInt64,
    deltas: Deltas<DeltaInt64>,
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
fn store_root(block: Block, store: StoreSetInt64) {
    store.set(
        block.number,
        format!("key.{}", block.number),
        &(block.number as i64),
    );
}

#[substreams::handlers::store]
fn store_depend(block: Block, store_root: StoreGetInt64, _store: StoreSetInt64) {
    let value = store_root.get_last("key.3");
    assert(block.number, true, value.is_some())
}

#[substreams::handlers::store]
fn store_depends_on_depend(
    block: Block,
    store_root: StoreGetInt64,
    _store_depend: StoreGetInt64,
    _store: StoreSetInt64,
) {
    let value = store_root.get_last("key.3");
    assert(block.number, true, value.is_some())
}

#[substreams::handlers::store]
fn setup_test_store_get_set_string(block: Block, store: StoreSetString) {
    store.set(
        1,
        format!("key:{}", block.number),
        &block.number.to_string(),
    )
}

#[substreams::handlers::map]
fn assert_test_store_get_set_string(
    block: Block,
    s: StoreGetString,
    deltas: Deltas<DeltaString>,
) -> Result<bool, errors::Error> {
    if deltas.deltas.len() != 1 {
        panic!("expected 1 deltas, got {}", deltas.deltas.len());
    }

    let value = s.get_last(format!("key:{}", block.number)).unwrap();
    assert(block.number, block.number.to_string(), value);

    let delta_0 = deltas.deltas.get(0).unwrap();
    assert(
        block.number,
        block.number.to_string(),
        delta_0.new_value.clone(),
    );

    Ok(true)
}

#[substreams::handlers::store]
fn setup_test_store_get_array_string(block: Block, s: StoreAppend<String>) {
    for i in 0..5 {
        s.append(1, format!("key:{}:{}", block.number, i), i.to_string())
    }
}

#[substreams::handlers::map]
fn assert_test_store_get_array_string(
    block: Block,
    s: StoreGetArray<String>,
    deltas: Deltas<DeltaArray<String>>,
) -> Result<bool, errors::Error> {
    if deltas.deltas.len() != 5 {
        panic!("expected 5 deltas, got {}", deltas.deltas.len());
    }

    for i in 0..5 {
        let value = s.get_last(format!("key:{}:{}", block.number, i)).unwrap();

        for elem in value {
            assert(block.number, i.to_string(), elem);
        }

        let delta_array = deltas.deltas.get(0).unwrap();
        for delta in delta_array.old_value.clone() {
            assert(block.number, i.to_string(), delta);
        }
    }

    Ok(true)
}

#[substreams::handlers::store]
fn setup_test_store_get_array_proto(block: Block, s: StoreAppend<Block>) {
    for _i in 0..5 {
        // append the block 5 times on the same key
        s.append(1, format!("key:{}", block.number), block.clone());
    }
}

#[substreams::handlers::map]
fn assert_test_store_get_array_proto(
    block: Block,
    s: StoreGetArray<Block>,
    deltas: Deltas<DeltaArray<Block>>,
) -> Result<bool, errors::Error> {
    if deltas.deltas.len() != 5 {
        panic!("expected 5 deltas, got {}", deltas.deltas.len());
    }

    for i in 0..5 {
        let blocks = s.get_last(format!("key:{}", block.number)).unwrap();
        assert(block.number, 5, blocks.len());

        for blk in blocks {
            assert(block.number, &block.id, &blk.id);
            assert(block.number, &block.number, &blk.number);
            assert(block.number, &block.step, &blk.step);
        }

        let delta = &deltas.deltas[i];
        if i == 0 {
            assert(block.number, 0, delta.old_value.len());
            assert(block.number, 1, delta.new_value.len());
        } else {
            assert(block.number, i, delta.old_value.len());
            assert(block.number, i + 1, delta.new_value.len());
        }
    }

    Ok(true)
}

#[substreams::handlers::store]
fn assert_all_test(
    _assert_test_store_delete_prefix: bool,
    _assert_test_store_add_bigint: bool,
    _assert_test_store_add_i64: bool,
    _assert_test_store_add_i64_deltas: bool,
    _assert_test_store_set_i64: bool,
    _assert_test_store_set_i64_deltas: bool,
) {
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
