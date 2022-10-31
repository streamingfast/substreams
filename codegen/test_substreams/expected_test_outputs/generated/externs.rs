use substreams::prelude::*;
use substreams::errors::Error;
use crate::pb;
use crate::generated::substreams::{Substreams, SubstreamsTrait};


#[no_mangle]
pub extern "C" fn map_block(
    block_ptr: *mut u8,
    block_len: usize,
) {
    substreams::register_panic_hook();
    let func = ||-> Result<pb::my_types_v1::Tests, Error>{
        
        let block: substreams_ethereum::pb::eth::v2::Block = substreams::proto::decode_ptr(block_ptr, block_len).unwrap();

        Substreams::map_block(block,
            
        )
    };
    let result = func();
    if result.is_err() {
        panic!("{:?}", &result.err().unwrap());
    }
    substreams::output(result.unwrap());
}

#[no_mangle]
pub extern "C" fn map_block_i64(
    block_ptr: *mut u8,
    block_len: usize,
) {
    substreams::register_panic_hook();
    let func = ||-> Result<i64, Error>{
        
        let block: substreams_ethereum::pb::eth::v2::Block = substreams::proto::decode_ptr(block_ptr, block_len).unwrap();

        Substreams::map_block_i64(block,
            
        )
    };
    let result = func();
    if result.is_err() {
        panic!("{:?}", &result.err().unwrap());
    }
    substreams::output(result.unwrap());
}

#[no_mangle]
pub extern "C" fn store_test(
    block_ptr: *mut u8,
    block_len: usize,
    map_block_ptr: *mut u8,
    map_block_len: usize,
) {
    substreams::register_panic_hook();
    let func = ||{
        
        let store: substreams::store::StoreSetProto<pb::my_types_v1::Test> = substreams::store::StoreSetProto::new();
        
        let block: substreams_ethereum::pb::eth::v2::Block = substreams::proto::decode_ptr(block_ptr, block_len).unwrap();
        let map_block: pb::my_types_v1::Tests = substreams::proto::decode_ptr(map_block_ptr, map_block_len).unwrap();

        Substreams::store_test(block,
            map_block,
            store,
        )
    };
    func()
}

#[no_mangle]
pub extern "C" fn store_append_string(
    block_ptr: *mut u8,
    block_len: usize,
) {
    substreams::register_panic_hook();
    let func = ||{
        
        let store: substreams::store::StoreAppend<String> = substreams::store::StoreAppend::new();
        
        let block: substreams_ethereum::pb::eth::v2::Block = substreams::proto::decode_ptr(block_ptr, block_len).unwrap();

        Substreams::store_append_string(block,
            store,
        )
    };
    func()
}

#[no_mangle]
pub extern "C" fn store_bigint(
    block_ptr: *mut u8,
    block_len: usize,
) {
    substreams::register_panic_hook();
    let func = ||{
        
        let store: substreams::store::StoreSetBigInt = substreams::store::StoreSetBigInt::new();
        
        let block: substreams_ethereum::pb::eth::v2::Block = substreams::proto::decode_ptr(block_ptr, block_len).unwrap();

        Substreams::store_bigint(block,
            store,
        )
    };
    func()
}

#[no_mangle]
pub extern "C" fn store_test2(
    block_ptr: *mut u8,
    block_len: usize,
    map_block_ptr: *mut u8,
    map_block_len: usize,
    store_test_ptr: u32,
    store_test_deltas_ptr: *mut u8,
    store_test_deltas_len: usize,
    map_block_i64_ptr: *mut u8,
    map_block_i64_len: usize,
    store_bigint_ptr: u32,
    store_bigint_deltas_ptr: *mut u8,
    store_bigint_deltas_len: usize,
    store_append_string_ptr: u32,
    store_append_string_deltas_ptr: *mut u8,
    store_append_string_deltas_len: usize,
) {
    substreams::register_panic_hook();
    let func = ||{
        
        let store: substreams::store::StoreSetProto<pb::my_types_v1::Test> = substreams::store::StoreSetProto::new();
        
        let block: substreams_ethereum::pb::eth::v2::Block = substreams::proto::decode_ptr(block_ptr, block_len).unwrap();
        let map_block: pb::my_types_v1::Tests = substreams::proto::decode_ptr(map_block_ptr, map_block_len).unwrap();
        let store_test: substreams::store::StoreGetProto<pb::my_types_v1::Test>  = substreams::store::StoreGetProto::new(store_test_ptr);
        let raw_store_test_deltas = substreams::proto::decode_ptr::<substreams::pb::substreams::StoreDeltas>(store_test_deltas_ptr, store_test_deltas_len).unwrap().deltas;
		let store_test_deltas: substreams::store::Deltas<substreams::store::DeltaProto<pb::my_types_v1::Test>> = substreams::store::Deltas::new(raw_store_test_deltas);
        let map_block_i64: i64 = substreams::proto::decode_ptr(map_block_i64_ptr, map_block_i64_len).unwrap();
        let store_bigint: substreams::store::StoreGetBigInt = substreams::store::StoreGetBigInt::new(store_bigint_ptr);
        let raw_store_bigint_deltas = substreams::proto::decode_ptr::<substreams::pb::substreams::StoreDeltas>(store_bigint_deltas_ptr, store_bigint_deltas_len).unwrap().deltas;
		let store_bigint_deltas: substreams::store::Deltas<substreams::store::DeltaBigInt> = substreams::store::Deltas::new(raw_store_bigint_deltas);
        let store_append_string: substreams::store::StoreGetRaw = substreams::store::StoreGetRaw::new(store_append_string_ptr);
        let raw_store_append_string_deltas = substreams::proto::decode_ptr::<substreams::pb::substreams::StoreDeltas>(store_append_string_deltas_ptr, store_append_string_deltas_len).unwrap().deltas;
		let store_append_string_deltas: substreams::store::Deltas<substreams::store::DeltaArray<String>> = substreams::store::Deltas::new(raw_store_append_string_deltas);

        Substreams::store_test2(block,
            map_block,
            store_test,
            store_test_deltas,
            map_block_i64,
            store_bigint,
            store_bigint_deltas,
            store_append_string,
            store_append_string_deltas,
            store,
        )
    };
    func()
}
