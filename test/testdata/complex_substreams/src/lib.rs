mod pb;

use crate::pb::test;
use crate::pb::keys;
use substreams::errors::Error;
use substreams::prelude::*;
use substreams::store::StoreAdd;
use substreams::store::StoreNew;
use crate::pb::test::Block;
use substreams::{store, Hex};



#[substreams::handlers::map]
fn test_index(blk: test::Block) -> Result<keys::Keys, substreams::errors::Error> {
    let mut keys = keys::Keys::default();
    if blk.number % 2 == 0 {
        keys.keys.push("even".to_string());
    } else {
        keys.keys.push("odd".to_string());
    }

    Ok(keys)
}

#[substreams::handlers::map]
fn assert_test_index(blk: test::Block, keys: keys::Keys) -> Result<test::Boolean, Error> {
    if blk.number % 2 == 0 {
        assert!(keys.keys[0] == "even"); 
        } else {
        assert!(keys.keys[0] == "odd"); 
    } 
    Ok(test::Boolean { result: true })
}


#[substreams::handlers::store]
fn test_first_store_init_20(block: test::Block, first_store: StoreAddInt64) {
    first_store.add(0, "block_counter", 1);
}

#[substreams::handlers::store]
fn test_second_store_init_30(block: test::Block, first_store: StoreGetInt64, second_store: StoreSetInt64) {
    let block_counter = first_store.get_last("block_counter").unwrap();
    second_store.set(0, format!("block_counter_from_first_store"), &block_counter)
}

#[substreams::handlers::store]
fn test_third_store_init_40(block: test::Block, second_store: StoreGetInt64, third_store: StoreSetInt64) {
    let block_counter = second_store.get_last("block_counter_from_first_store").unwrap();
    third_store.set(0, format!("block_counter_from_second_store"), &block_counter)
}


#[substreams::handlers::map]
fn assert_test_first_store_init_20(block: test::Block, first_store: StoreGetInt64) -> Result<test::Boolean, Error> {
    let block_counter = first_store.get_last("block_counter");

    if block.number < 20 {
        assert!(block_counter.is_none());
        return Ok(test::Boolean { result: true })
    }

    assert_eq!(block_counter.unwrap(), (block.number - 19) as i64);
    Ok(test::Boolean { result: true })
}

#[substreams::handlers::map]
fn assert_test_first_store_deltas_init_20(block: test::Block, first_store: store::Deltas<DeltaInt64>) -> Result<test::Boolean, Error> {
    let mut block_counter = None;

    first_store
    .deltas
    .iter()
    .for_each(|delta| match delta.key.as_str() {
        "block_counter" => block_counter = Some(delta.new_value),
        x => panic!("unhandled key {}", x),
    });

    if block.number < 20 {
        assert!(block_counter.is_none());
        return Ok(test::Boolean { result: true })
    }

    assert_eq!(block_counter.unwrap(), (block.number - 19) as i64);
    Ok(test::Boolean { result: true })
}

#[substreams::handlers::map]
fn assert_test_second_store_init_20(block: test::Block, second_store: StoreGetInt64) -> Result<test::Boolean, Error> {
    let block_counter = second_store.get_last("block_counter_from_first_store");

    if block.number < 30 {
        assert!(block_counter.is_none());
        return Ok(test::Boolean { result: true })
    }

    assert_eq!(block_counter.unwrap(), (block.number - 19) as i64);
    Ok(test::Boolean { result: true })
}

#[substreams::handlers::map]
fn assert_test_second_store_deltas_init_20(block: test::Block, second_store: store::Deltas<DeltaInt64>) -> Result<test::Boolean, Error> {
    let mut block_counter = None;

    second_store
    .deltas
    .iter()
    .for_each(|delta| match delta.key.as_str() {
        "block_counter_from_first_store" => block_counter = Some(delta.new_value),
        x => panic!("unhandled key {}", x),
    });

    if block.number < 30 {
        assert!(block_counter.is_none());
        return Ok(test::Boolean { result: true })
    }

    assert_eq!(block_counter.unwrap(), (block.number - 19) as i64);
    Ok(test::Boolean { result: true })
}

#[substreams::handlers::map]
fn assert_test_third_store_init_20(block: test::Block, third_store: StoreGetInt64) -> Result<test::Boolean, Error> {
    let block_counter = third_store.get_last("block_counter_from_second_store");

    if block.number < 40 {
        assert!(block_counter.is_none());
        return Ok(test::Boolean { result: true })
    }

    assert_eq!(block_counter.unwrap(), (block.number - 19) as i64);
    Ok(test::Boolean { result: true })
}

#[substreams::handlers::map]
fn assert_test_third_store_deltas_init_20(block: test::Block, third_store: store::Deltas<DeltaInt64>) -> Result<test::Boolean, Error> {
    let mut block_counter = None;

    third_store
    .deltas
    .iter()
    .for_each(|delta| match delta.key.as_str() {
        "block_counter_from_second_store" => block_counter = Some(delta.new_value),
        x => panic!("unhandled key {}", x),
    });

    if block.number < 40 {
        assert!(block_counter.is_none());
        return Ok(test::Boolean { result: true })
    }

    assert_eq!(block_counter.unwrap(), (block.number - 19) as i64);
    Ok(test::Boolean { result: true })
}

#[substreams::handlers::map]
fn all_test_assert_init_20(result_one: test::Boolean, result_two: test::Boolean, result_three: test::Boolean, result_fourth: test::Boolean, result_fifth: test::Boolean, result_sixth: test::Boolean) -> Result<test::Boolean, Error> {
    //
    Ok(test::Boolean { result: true })
}


#[substreams::handlers::map]
fn test_map_output_init_50(block: test::Block, third_store: StoreGetInt64) -> Result<test::MapResult, substreams::errors::Error> {
    let fake_block_number = third_store.get_last("block_counter_from_second_store").unwrap() as u64;

    let out = test::MapResult {
        block_number: fake_block_number,
        block_hash: block.id,
    };

    Ok(out)
}



