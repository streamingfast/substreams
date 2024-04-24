mod pb;

use crate::pb::test;
use crate::pb::test::Block;
use crate::pb::keys;
use substreams::errors::Error;
use substreams::store::{
    StoreSetInt64, StoreGetInt64, StoreAddInt64
};



#[substreams::handlers::map]
fn test_index_block(blk: test::Block) -> Result<keys::Keys, substreams::errors::Error> {
    let mut keys = keys::Keys::default();
    if blk.number % 2 == 0 {
        keys.keys.push("even".to_string());
    } else {
        keys.keys.push("odd".to_string());
    }
    Ok(keys)
}


#[substreams::handlers::store]
fn test_first_store(block: test::Block, first_store: StoreAddInt64) {
    first_store.add(0, "block_counter", 1);
}

#[substreams::handlers::store]
fn test_second_store(block: test::Block, first_store: StoreGetInt64, second_store: StoreSetInt64) {
    let block_counter = first_store.get_last(0, "block_counter").unwrap();
    second_store.set(0, format!("block_counter_from_first_store"), block_counter)
}

#[substreams::handlers::store]
fn test_third_store(block: test::Block, second_store: StoreGetInt64, third_store: StoreSetInt64) {
    let block_counter = second_store.get_last(0, "block_counter_from_first_store").unwrap();
    third_store.set(0, format!("block_counter_from_second_store"), block_counter)
}


#[substreams::handlers::map]
fn assert_test_first_store(block: test::Block, first_store: StoreGetInt64) -> Result<test::Boolean, substreams::errors::Error> {
    if block.number < 20 {
        // ??
    } else {
        let block_counter = first_store.get_last(0, "block_counter").unwrap();
        assert_eq!(block_counter, block.number - 20);
        Ok(test::Boolean { result: true })
    }
}

#[substreams::handlers::map]
fn assert_test_second_store(block: test::Block, second_store: StoreGetInt64) -> Result<test::Boolean, substreams::errors::Error> {
    if block.number < 30 {
        // Shoud not have any input 
    } else {
        let block_counter = second_store.get_last(0, "block_counter").unwrap();
        assert_eq!(block_counter, block.number - 20);
        Ok(test::Boolean { result: true })
    }
}

#[substreams::handlers::map]
fn assert_test_third_store(block: test::Block, third_store: StoreGetInt64) -> Result<test::Boolean, substreams::errors::Error> {
    if block.number < 40 {
        // Shoud not have any input 
        
    } else {
        let block_counter = third_store.get_last(0, "block_counter").unwrap();
        assert_eq!(block_counter, block.number - 20);
        Ok(test::Boolean { result: true })
    }
}

#[substreams::handlers::map]
fn test_map_output(block: test::Block, third_store: StoreGetInt64) -> Result<test::MapResult, substreams::errors::Error> {
    let fake_block_number = third_store.get_last(0, "block_counter_from_second_store");

    let out = test::MapResult {
        block_number: fake_block_number,
        block_hash: block.id,
    };

    Ok(out)
}



