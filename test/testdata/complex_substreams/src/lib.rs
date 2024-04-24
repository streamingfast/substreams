mod pb;

use crate::pb::test;
//use crate::pb::test::Block;
use crate::pb::keys;
use substreams::errors::Error;
use substreams::prelude::*;
use substreams::store::StoreAdd;
use substreams::store::StoreNew;
//use substreams::store::{
//    StoreSetInt64, StoreGetInt64, StoreAddInt64
//};



#[substreams::handlers::map]
fn index_block(blk: test::Block) -> Result<keys::Keys, substreams::errors::Error> {
    let mut keys = keys::Keys::default();
    if blk.number % 2 == 0 {
        keys.keys.push("even".to_string());
    } else {
        keys.keys.push("odd".to_string());
    }
    Ok(keys)
}


#[substreams::handlers::store]
fn first_store(block: test::Block, first_store: StoreAddInt64) {
    first_store.add(0, "block_counter", 1);
}

#[substreams::handlers::store]
fn test_second_store(block: test::Block, first_store: StoreGetInt64, second_store: StoreSetInt64) {
    let block_counter = first_store.get_last("block_counter").unwrap();
    second_store.set(0, format!("block_counter_from_first_store"), &block_counter)
}

#[substreams::handlers::store]
fn test_third_store(block: test::Block, second_store: StoreGetInt64, third_store: StoreSetInt64) {
    let block_counter = second_store.get_last( "block_counter_from_first_store").unwrap();
    third_store.set(0, format!("block_counter_from_second_store"), &block_counter)
}


#[substreams::handlers::map]
fn assert_test_first_store(block: test::Block, first_store: StoreGetInt64) -> Result<test::Boolean, Error> {
    assert!(block.number >= 20); 
    let block_counter = first_store.get_last("block_counter").unwrap();
    assert_eq!(block_counter, (block.number - 20) as i64);
    Ok(test::Boolean { result: true })
}

#[substreams::handlers::map]
fn assert_test_second_store(block: test::Block, second_store: StoreGetInt64) -> Result<test::Boolean, Error> {
    assert!(block.number >= 30);
    let block_counter = second_store.get_last("block_counter").unwrap();
    assert_eq!(block_counter, (block.number - 20) as i64);
    Ok(test::Boolean { result: true })
}

#[substreams::handlers::map]
fn assert_test_third_store(block: test::Block, third_store: StoreGetInt64) -> Result<test::Boolean, Error> {
    assert!(block.number >= 40); 
    let block_counter = third_store.get_last("block_counter").unwrap();
    assert_eq!(block_counter, (block.number - 20) as i64);
    Ok(test::Boolean { result: true })
}

#[substreams::handlers::map]
fn test_map_output(block: test::Block, third_store: StoreGetInt64) -> Result<test::MapResult, substreams::errors::Error> {
    let fake_block_number = third_store.get_last("block_counter_from_second_store").unwrap() as u64;

    let out = test::MapResult {
        block_number: fake_block_number,
        block_hash: block.id,
    };

    Ok(out)
}



