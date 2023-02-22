use substreams::prelude::*;
use substreams::errors::Error;
use crate::pb;
use crate::generated::substreams::{Substreams, SubstreamsTrait};


#[no_mangle]
pub extern "C" fn test_map(
    params_ptr: *mut u8,
    params_len: usize,
    block_ptr: *mut u8,
    block_len: usize,
) {
    substreams::register_panic_hook();
    let func = ||-> Result<pb::test::MapResult, Error>{
        
        let params: String = std::mem::ManuallyDrop::new(unsafe { String::from_raw_parts(params_ptr, params_len, params_len) }).to_string();
        let block: pb::test::Block = substreams::proto::decode_ptr(block_ptr, block_len).unwrap();

        Substreams::test_map(params,
            block,
            
        )
    };
    let result = func();
    if result.is_err() {
        panic!("{:?}", &result.err().unwrap());
    }
    substreams::output(result.unwrap());
}

#[no_mangle]
pub extern "C" fn test_store_proto(
    test_map_ptr: *mut u8,
    test_map_len: usize,
) {
    substreams::register_panic_hook();
    let func = ||{
        
        let store: substreams::store::StoreSetProto<pb::test::MapResult> = substreams::store::StoreSetProto::new();
        
        let test_map: pb::test::MapResult = substreams::proto::decode_ptr(test_map_ptr, test_map_len).unwrap();

        Substreams::test_store_proto(test_map,
            store,
        )
    };
    func()
}

#[no_mangle]
pub extern "C" fn test_store_delete_prefix(
    block_ptr: *mut u8,
    block_len: usize,
) {
    substreams::register_panic_hook();
    let func = ||{
        
        let store: substreams::store::StoreSetInt64 = substreams::store::StoreSetInt64::new();
        
        let block: pb::test::Block = substreams::proto::decode_ptr(block_ptr, block_len).unwrap();

        Substreams::test_store_delete_prefix(block,
            store,
        )
    };
    func()
}

#[no_mangle]
pub extern "C" fn assert_test_store_delete_prefix(
    block_ptr: *mut u8,
    block_len: usize,
    test_store_delete_prefix_ptr: u32,
) {
    substreams::register_panic_hook();
    let func = ||-> Result<pb::test::Boolean, Error>{
        
        let block: pb::test::Block = substreams::proto::decode_ptr(block_ptr, block_len).unwrap();
        let test_store_delete_prefix: substreams::store::StoreGetInt64 = substreams::store::StoreGetInt64::new(test_store_delete_prefix_ptr);

        Substreams::assert_test_store_delete_prefix(block,
            test_store_delete_prefix,
            
        )
    };
    let result = func();
    if result.is_err() {
        panic!("{:?}", &result.err().unwrap());
    }
    substreams::output(result.unwrap());
}

#[no_mangle]
pub extern "C" fn setup_test_store_add_i64(
    block_ptr: *mut u8,
    block_len: usize,
) {
    substreams::register_panic_hook();
    let func = ||{
        
        let store: substreams::store::StoreAddInt64 = substreams::store::StoreAddInt64::new();
        
        let block: pb::test::Block = substreams::proto::decode_ptr(block_ptr, block_len).unwrap();

        Substreams::setup_test_store_add_i64(block,
            store,
        )
    };
    func()
}

#[no_mangle]
pub extern "C" fn assert_test_store_add_i64(
    block_ptr: *mut u8,
    block_len: usize,
    setup_test_store_add_i64_ptr: u32,
) {
    substreams::register_panic_hook();
    let func = ||-> Result<pb::test::Boolean, Error>{
        
        let block: pb::test::Block = substreams::proto::decode_ptr(block_ptr, block_len).unwrap();
        let setup_test_store_add_i64: substreams::store::StoreGetInt64 = substreams::store::StoreGetInt64::new(setup_test_store_add_i64_ptr);

        Substreams::assert_test_store_add_i64(block,
            setup_test_store_add_i64,
            
        )
    };
    let result = func();
    if result.is_err() {
        panic!("{:?}", &result.err().unwrap());
    }
    substreams::output(result.unwrap());
}

#[no_mangle]
pub extern "C" fn assert_test_store_add_i64_deltas(
    block_ptr: *mut u8,
    block_len: usize,
    setup_test_store_add_i64_ptr: u32,
    setup_test_store_add_i64_deltas_ptr: *mut u8,
    setup_test_store_add_i64_deltas_len: usize,
) {
    substreams::register_panic_hook();
    let func = ||-> Result<pb::test::Boolean, Error>{
        
        let block: pb::test::Block = substreams::proto::decode_ptr(block_ptr, block_len).unwrap();
        let setup_test_store_add_i64: substreams::store::StoreGetInt64 = substreams::store::StoreGetInt64::new(setup_test_store_add_i64_ptr);
        let raw_setup_test_store_add_i64_deltas = substreams::proto::decode_ptr::<substreams::pb::substreams::StoreDeltas>(setup_test_store_add_i64_deltas_ptr, setup_test_store_add_i64_deltas_len).unwrap().deltas;
		let setup_test_store_add_i64_deltas: substreams::store::Deltas<substreams::store::DeltaInt64> = substreams::store::Deltas::new(raw_setup_test_store_add_i64_deltas);

        Substreams::assert_test_store_add_i64_deltas(block,
            setup_test_store_add_i64,
            setup_test_store_add_i64_deltas,
            
        )
    };
    let result = func();
    if result.is_err() {
        panic!("{:?}", &result.err().unwrap());
    }
    substreams::output(result.unwrap());
}

#[no_mangle]
pub extern "C" fn setup_test_store_set_i64(
    block_ptr: *mut u8,
    block_len: usize,
) {
    substreams::register_panic_hook();
    let func = ||{
        
        let store: substreams::store::StoreSetInt64 = substreams::store::StoreSetInt64::new();
        
        let block: pb::test::Block = substreams::proto::decode_ptr(block_ptr, block_len).unwrap();

        Substreams::setup_test_store_set_i64(block,
            store,
        )
    };
    func()
}

#[no_mangle]
pub extern "C" fn assert_test_store_set_i64(
    block_ptr: *mut u8,
    block_len: usize,
    setup_test_store_set_i64_ptr: u32,
) {
    substreams::register_panic_hook();
    let func = ||-> Result<pb::test::Boolean, Error>{
        
        let block: pb::test::Block = substreams::proto::decode_ptr(block_ptr, block_len).unwrap();
        let setup_test_store_set_i64: substreams::store::StoreGetInt64 = substreams::store::StoreGetInt64::new(setup_test_store_set_i64_ptr);

        Substreams::assert_test_store_set_i64(block,
            setup_test_store_set_i64,
            
        )
    };
    let result = func();
    if result.is_err() {
        panic!("{:?}", &result.err().unwrap());
    }
    substreams::output(result.unwrap());
}

#[no_mangle]
pub extern "C" fn assert_test_store_set_i64_deltas(
    block_ptr: *mut u8,
    block_len: usize,
    setup_test_store_set_i64_ptr: u32,
    setup_test_store_set_i64_deltas_ptr: *mut u8,
    setup_test_store_set_i64_deltas_len: usize,
) {
    substreams::register_panic_hook();
    let func = ||-> Result<pb::test::Boolean, Error>{
        
        let block: pb::test::Block = substreams::proto::decode_ptr(block_ptr, block_len).unwrap();
        let setup_test_store_set_i64: substreams::store::StoreGetInt64 = substreams::store::StoreGetInt64::new(setup_test_store_set_i64_ptr);
        let raw_setup_test_store_set_i64_deltas = substreams::proto::decode_ptr::<substreams::pb::substreams::StoreDeltas>(setup_test_store_set_i64_deltas_ptr, setup_test_store_set_i64_deltas_len).unwrap().deltas;
		let setup_test_store_set_i64_deltas: substreams::store::Deltas<substreams::store::DeltaInt64> = substreams::store::Deltas::new(raw_setup_test_store_set_i64_deltas);

        Substreams::assert_test_store_set_i64_deltas(block,
            setup_test_store_set_i64,
            setup_test_store_set_i64_deltas,
            
        )
    };
    let result = func();
    if result.is_err() {
        panic!("{:?}", &result.err().unwrap());
    }
    substreams::output(result.unwrap());
}

#[no_mangle]
pub extern "C" fn setup_test_store_set_if_not_exists_i64(
    block_ptr: *mut u8,
    block_len: usize,
) {
    substreams::register_panic_hook();
    let func = ||{
        
        let store: substreams::store::StoreSetIfNotExistsInt64 = substreams::store::StoreSetIfNotExistsInt64::new();
        
        let block: pb::test::Block = substreams::proto::decode_ptr(block_ptr, block_len).unwrap();

        Substreams::setup_test_store_set_if_not_exists_i64(block,
            store,
        )
    };
    func()
}

#[no_mangle]
pub extern "C" fn assert_test_store_set_if_not_exists_i64(
    block_ptr: *mut u8,
    block_len: usize,
    setup_test_store_set_if_not_exists_i64_ptr: u32,
) {
    substreams::register_panic_hook();
    let func = ||-> Result<pb::test::Boolean, Error>{
        
        let block: pb::test::Block = substreams::proto::decode_ptr(block_ptr, block_len).unwrap();
        let setup_test_store_set_if_not_exists_i64: substreams::store::StoreGetInt64 = substreams::store::StoreGetInt64::new(setup_test_store_set_if_not_exists_i64_ptr);

        Substreams::assert_test_store_set_if_not_exists_i64(block,
            setup_test_store_set_if_not_exists_i64,
            
        )
    };
    let result = func();
    if result.is_err() {
        panic!("{:?}", &result.err().unwrap());
    }
    substreams::output(result.unwrap());
}

#[no_mangle]
pub extern "C" fn assert_test_store_set_if_not_exists_i64_deltas(
    block_ptr: *mut u8,
    block_len: usize,
    setup_test_store_set_if_not_exists_i64_ptr: u32,
    setup_test_store_set_if_not_exists_i64_deltas_ptr: *mut u8,
    setup_test_store_set_if_not_exists_i64_deltas_len: usize,
) {
    substreams::register_panic_hook();
    let func = ||-> Result<pb::test::Boolean, Error>{
        
        let block: pb::test::Block = substreams::proto::decode_ptr(block_ptr, block_len).unwrap();
        let setup_test_store_set_if_not_exists_i64: substreams::store::StoreGetInt64 = substreams::store::StoreGetInt64::new(setup_test_store_set_if_not_exists_i64_ptr);
        let raw_setup_test_store_set_if_not_exists_i64_deltas = substreams::proto::decode_ptr::<substreams::pb::substreams::StoreDeltas>(setup_test_store_set_if_not_exists_i64_deltas_ptr, setup_test_store_set_if_not_exists_i64_deltas_len).unwrap().deltas;
		let setup_test_store_set_if_not_exists_i64_deltas: substreams::store::Deltas<substreams::store::DeltaInt64> = substreams::store::Deltas::new(raw_setup_test_store_set_if_not_exists_i64_deltas);

        Substreams::assert_test_store_set_if_not_exists_i64_deltas(block,
            setup_test_store_set_if_not_exists_i64,
            setup_test_store_set_if_not_exists_i64_deltas,
            
        )
    };
    let result = func();
    if result.is_err() {
        panic!("{:?}", &result.err().unwrap());
    }
    substreams::output(result.unwrap());
}

#[no_mangle]
pub extern "C" fn setup_test_store_min_i64(
    block_ptr: *mut u8,
    block_len: usize,
) {
    substreams::register_panic_hook();
    let func = ||{
        
        let store: substreams::store::StoreMinInt64 = substreams::store::StoreMinInt64::new();
        
        let block: pb::test::Block = substreams::proto::decode_ptr(block_ptr, block_len).unwrap();

        Substreams::setup_test_store_min_i64(block,
            store,
        )
    };
    func()
}

#[no_mangle]
pub extern "C" fn assert_test_store_min_i64(
    block_ptr: *mut u8,
    block_len: usize,
    setup_test_store_min_i64_ptr: u32,
) {
    substreams::register_panic_hook();
    let func = ||-> Result<pb::test::Boolean, Error>{
        
        let block: pb::test::Block = substreams::proto::decode_ptr(block_ptr, block_len).unwrap();
        let setup_test_store_min_i64: substreams::store::StoreGetInt64 = substreams::store::StoreGetInt64::new(setup_test_store_min_i64_ptr);

        Substreams::assert_test_store_min_i64(block,
            setup_test_store_min_i64,
            
        )
    };
    let result = func();
    if result.is_err() {
        panic!("{:?}", &result.err().unwrap());
    }
    substreams::output(result.unwrap());
}

#[no_mangle]
pub extern "C" fn assert_test_store_min_i64_deltas(
    block_ptr: *mut u8,
    block_len: usize,
    setup_test_store_min_i64_ptr: u32,
    setup_test_store_min_i64_deltas_ptr: *mut u8,
    setup_test_store_min_i64_deltas_len: usize,
) {
    substreams::register_panic_hook();
    let func = ||-> Result<pb::test::Boolean, Error>{
        
        let block: pb::test::Block = substreams::proto::decode_ptr(block_ptr, block_len).unwrap();
        let setup_test_store_min_i64: substreams::store::StoreGetInt64 = substreams::store::StoreGetInt64::new(setup_test_store_min_i64_ptr);
        let raw_setup_test_store_min_i64_deltas = substreams::proto::decode_ptr::<substreams::pb::substreams::StoreDeltas>(setup_test_store_min_i64_deltas_ptr, setup_test_store_min_i64_deltas_len).unwrap().deltas;
		let setup_test_store_min_i64_deltas: substreams::store::Deltas<substreams::store::DeltaInt64> = substreams::store::Deltas::new(raw_setup_test_store_min_i64_deltas);

        Substreams::assert_test_store_min_i64_deltas(block,
            setup_test_store_min_i64,
            setup_test_store_min_i64_deltas,
            
        )
    };
    let result = func();
    if result.is_err() {
        panic!("{:?}", &result.err().unwrap());
    }
    substreams::output(result.unwrap());
}

#[no_mangle]
pub extern "C" fn setup_test_store_max_i64(
    block_ptr: *mut u8,
    block_len: usize,
) {
    substreams::register_panic_hook();
    let func = ||{
        
        let store: substreams::store::StoreMaxInt64 = substreams::store::StoreMaxInt64::new();
        
        let block: pb::test::Block = substreams::proto::decode_ptr(block_ptr, block_len).unwrap();

        Substreams::setup_test_store_max_i64(block,
            store,
        )
    };
    func()
}

#[no_mangle]
pub extern "C" fn assert_test_store_max_i64(
    block_ptr: *mut u8,
    block_len: usize,
    setup_test_store_max_i64_ptr: u32,
) {
    substreams::register_panic_hook();
    let func = ||-> Result<pb::test::Boolean, Error>{
        
        let block: pb::test::Block = substreams::proto::decode_ptr(block_ptr, block_len).unwrap();
        let setup_test_store_max_i64: substreams::store::StoreGetInt64 = substreams::store::StoreGetInt64::new(setup_test_store_max_i64_ptr);

        Substreams::assert_test_store_max_i64(block,
            setup_test_store_max_i64,
            
        )
    };
    let result = func();
    if result.is_err() {
        panic!("{:?}", &result.err().unwrap());
    }
    substreams::output(result.unwrap());
}

#[no_mangle]
pub extern "C" fn assert_test_store_max_i64_deltas(
    block_ptr: *mut u8,
    block_len: usize,
    setup_test_store_max_i64_ptr: u32,
    setup_test_store_max_i64_deltas_ptr: *mut u8,
    setup_test_store_max_i64_deltas_len: usize,
) {
    substreams::register_panic_hook();
    let func = ||-> Result<pb::test::Boolean, Error>{
        
        let block: pb::test::Block = substreams::proto::decode_ptr(block_ptr, block_len).unwrap();
        let setup_test_store_max_i64: substreams::store::StoreGetInt64 = substreams::store::StoreGetInt64::new(setup_test_store_max_i64_ptr);
        let raw_setup_test_store_max_i64_deltas = substreams::proto::decode_ptr::<substreams::pb::substreams::StoreDeltas>(setup_test_store_max_i64_deltas_ptr, setup_test_store_max_i64_deltas_len).unwrap().deltas;
		let setup_test_store_max_i64_deltas: substreams::store::Deltas<substreams::store::DeltaInt64> = substreams::store::Deltas::new(raw_setup_test_store_max_i64_deltas);

        Substreams::assert_test_store_max_i64_deltas(block,
            setup_test_store_max_i64,
            setup_test_store_max_i64_deltas,
            
        )
    };
    let result = func();
    if result.is_err() {
        panic!("{:?}", &result.err().unwrap());
    }
    substreams::output(result.unwrap());
}

#[no_mangle]
pub extern "C" fn setup_test_store_add_float64(
    block_ptr: *mut u8,
    block_len: usize,
) {
    substreams::register_panic_hook();
    let func = ||{
        
        let store: substreams::store::StoreAddFloat64 = substreams::store::StoreAddFloat64::new();
        
        let block: pb::test::Block = substreams::proto::decode_ptr(block_ptr, block_len).unwrap();

        Substreams::setup_test_store_add_float64(block,
            store,
        )
    };
    func()
}

#[no_mangle]
pub extern "C" fn assert_test_store_add_float64(
    block_ptr: *mut u8,
    block_len: usize,
    setup_test_store_add_float64_ptr: u32,
) {
    substreams::register_panic_hook();
    let func = ||-> Result<pb::test::Boolean, Error>{
        
        let block: pb::test::Block = substreams::proto::decode_ptr(block_ptr, block_len).unwrap();
        let setup_test_store_add_float64: substreams::store::StoreGetFloat64 = substreams::store::StoreGetFloat64::new(setup_test_store_add_float64_ptr);

        Substreams::assert_test_store_add_float64(block,
            setup_test_store_add_float64,
            
        )
    };
    let result = func();
    if result.is_err() {
        panic!("{:?}", &result.err().unwrap());
    }
    substreams::output(result.unwrap());
}

#[no_mangle]
pub extern "C" fn assert_test_store_add_float64_deltas(
    block_ptr: *mut u8,
    block_len: usize,
    setup_test_store_add_float64_ptr: u32,
    setup_test_store_add_float64_deltas_ptr: *mut u8,
    setup_test_store_add_float64_deltas_len: usize,
) {
    substreams::register_panic_hook();
    let func = ||-> Result<pb::test::Boolean, Error>{
        
        let block: pb::test::Block = substreams::proto::decode_ptr(block_ptr, block_len).unwrap();
        let setup_test_store_add_float64: substreams::store::StoreGetFloat64 = substreams::store::StoreGetFloat64::new(setup_test_store_add_float64_ptr);
        let raw_setup_test_store_add_float64_deltas = substreams::proto::decode_ptr::<substreams::pb::substreams::StoreDeltas>(setup_test_store_add_float64_deltas_ptr, setup_test_store_add_float64_deltas_len).unwrap().deltas;
		let setup_test_store_add_float64_deltas: substreams::store::Deltas<substreams::store::DeltaFloat64> = substreams::store::Deltas::new(raw_setup_test_store_add_float64_deltas);

        Substreams::assert_test_store_add_float64_deltas(block,
            setup_test_store_add_float64,
            setup_test_store_add_float64_deltas,
            
        )
    };
    let result = func();
    if result.is_err() {
        panic!("{:?}", &result.err().unwrap());
    }
    substreams::output(result.unwrap());
}

#[no_mangle]
pub extern "C" fn setup_test_store_set_float64(
    block_ptr: *mut u8,
    block_len: usize,
) {
    substreams::register_panic_hook();
    let func = ||{
        
        let store: substreams::store::StoreSetFloat64 = substreams::store::StoreSetFloat64::new();
        
        let block: pb::test::Block = substreams::proto::decode_ptr(block_ptr, block_len).unwrap();

        Substreams::setup_test_store_set_float64(block,
            store,
        )
    };
    func()
}

#[no_mangle]
pub extern "C" fn assert_test_store_set_float64(
    block_ptr: *mut u8,
    block_len: usize,
    setup_test_store_set_float64_ptr: u32,
) {
    substreams::register_panic_hook();
    let func = ||-> Result<pb::test::Boolean, Error>{
        
        let block: pb::test::Block = substreams::proto::decode_ptr(block_ptr, block_len).unwrap();
        let setup_test_store_set_float64: substreams::store::StoreGetFloat64 = substreams::store::StoreGetFloat64::new(setup_test_store_set_float64_ptr);

        Substreams::assert_test_store_set_float64(block,
            setup_test_store_set_float64,
            
        )
    };
    let result = func();
    if result.is_err() {
        panic!("{:?}", &result.err().unwrap());
    }
    substreams::output(result.unwrap());
}

#[no_mangle]
pub extern "C" fn assert_test_store_set_float64_deltas(
    block_ptr: *mut u8,
    block_len: usize,
    setup_test_store_set_float64_ptr: u32,
    setup_test_store_set_float64_deltas_ptr: *mut u8,
    setup_test_store_set_float64_deltas_len: usize,
) {
    substreams::register_panic_hook();
    let func = ||-> Result<pb::test::Boolean, Error>{
        
        let block: pb::test::Block = substreams::proto::decode_ptr(block_ptr, block_len).unwrap();
        let setup_test_store_set_float64: substreams::store::StoreGetFloat64 = substreams::store::StoreGetFloat64::new(setup_test_store_set_float64_ptr);
        let raw_setup_test_store_set_float64_deltas = substreams::proto::decode_ptr::<substreams::pb::substreams::StoreDeltas>(setup_test_store_set_float64_deltas_ptr, setup_test_store_set_float64_deltas_len).unwrap().deltas;
		let setup_test_store_set_float64_deltas: substreams::store::Deltas<substreams::store::DeltaFloat64> = substreams::store::Deltas::new(raw_setup_test_store_set_float64_deltas);

        Substreams::assert_test_store_set_float64_deltas(block,
            setup_test_store_set_float64,
            setup_test_store_set_float64_deltas,
            
        )
    };
    let result = func();
    if result.is_err() {
        panic!("{:?}", &result.err().unwrap());
    }
    substreams::output(result.unwrap());
}

#[no_mangle]
pub extern "C" fn setup_test_store_set_if_not_exists_float64(
    block_ptr: *mut u8,
    block_len: usize,
) {
    substreams::register_panic_hook();
    let func = ||{
        
        let store: substreams::store::StoreSetIfNotExistsFloat64 = substreams::store::StoreSetIfNotExistsFloat64::new();
        
        let block: pb::test::Block = substreams::proto::decode_ptr(block_ptr, block_len).unwrap();

        Substreams::setup_test_store_set_if_not_exists_float64(block,
            store,
        )
    };
    func()
}

#[no_mangle]
pub extern "C" fn assert_test_store_set_if_not_exists_float64(
    block_ptr: *mut u8,
    block_len: usize,
    setup_test_store_set_if_not_exists_float64_ptr: u32,
) {
    substreams::register_panic_hook();
    let func = ||-> Result<pb::test::Boolean, Error>{
        
        let block: pb::test::Block = substreams::proto::decode_ptr(block_ptr, block_len).unwrap();
        let setup_test_store_set_if_not_exists_float64: substreams::store::StoreGetFloat64 = substreams::store::StoreGetFloat64::new(setup_test_store_set_if_not_exists_float64_ptr);

        Substreams::assert_test_store_set_if_not_exists_float64(block,
            setup_test_store_set_if_not_exists_float64,
            
        )
    };
    let result = func();
    if result.is_err() {
        panic!("{:?}", &result.err().unwrap());
    }
    substreams::output(result.unwrap());
}

#[no_mangle]
pub extern "C" fn assert_test_store_set_if_not_exists_float64_deltas(
    block_ptr: *mut u8,
    block_len: usize,
    setup_test_store_set_if_not_exists_float64_ptr: u32,
    setup_test_store_set_if_not_exists_float64_deltas_ptr: *mut u8,
    setup_test_store_set_if_not_exists_float64_deltas_len: usize,
) {
    substreams::register_panic_hook();
    let func = ||-> Result<pb::test::Boolean, Error>{
        
        let block: pb::test::Block = substreams::proto::decode_ptr(block_ptr, block_len).unwrap();
        let setup_test_store_set_if_not_exists_float64: substreams::store::StoreGetFloat64 = substreams::store::StoreGetFloat64::new(setup_test_store_set_if_not_exists_float64_ptr);
        let raw_setup_test_store_set_if_not_exists_float64_deltas = substreams::proto::decode_ptr::<substreams::pb::substreams::StoreDeltas>(setup_test_store_set_if_not_exists_float64_deltas_ptr, setup_test_store_set_if_not_exists_float64_deltas_len).unwrap().deltas;
		let setup_test_store_set_if_not_exists_float64_deltas: substreams::store::Deltas<substreams::store::DeltaFloat64> = substreams::store::Deltas::new(raw_setup_test_store_set_if_not_exists_float64_deltas);

        Substreams::assert_test_store_set_if_not_exists_float64_deltas(block,
            setup_test_store_set_if_not_exists_float64,
            setup_test_store_set_if_not_exists_float64_deltas,
            
        )
    };
    let result = func();
    if result.is_err() {
        panic!("{:?}", &result.err().unwrap());
    }
    substreams::output(result.unwrap());
}

#[no_mangle]
pub extern "C" fn setup_test_store_min_float64(
    block_ptr: *mut u8,
    block_len: usize,
) {
    substreams::register_panic_hook();
    let func = ||{
        
        let store: substreams::store::StoreMinFloat64 = substreams::store::StoreMinFloat64::new();
        
        let block: pb::test::Block = substreams::proto::decode_ptr(block_ptr, block_len).unwrap();

        Substreams::setup_test_store_min_float64(block,
            store,
        )
    };
    func()
}

#[no_mangle]
pub extern "C" fn assert_test_store_min_float64(
    block_ptr: *mut u8,
    block_len: usize,
    setup_test_store_min_float64_ptr: u32,
) {
    substreams::register_panic_hook();
    let func = ||-> Result<pb::test::Boolean, Error>{
        
        let block: pb::test::Block = substreams::proto::decode_ptr(block_ptr, block_len).unwrap();
        let setup_test_store_min_float64: substreams::store::StoreGetFloat64 = substreams::store::StoreGetFloat64::new(setup_test_store_min_float64_ptr);

        Substreams::assert_test_store_min_float64(block,
            setup_test_store_min_float64,
            
        )
    };
    let result = func();
    if result.is_err() {
        panic!("{:?}", &result.err().unwrap());
    }
    substreams::output(result.unwrap());
}

#[no_mangle]
pub extern "C" fn assert_test_store_min_float64_deltas(
    block_ptr: *mut u8,
    block_len: usize,
    setup_test_store_min_float64_ptr: u32,
    setup_test_store_min_float64_deltas_ptr: *mut u8,
    setup_test_store_min_float64_deltas_len: usize,
) {
    substreams::register_panic_hook();
    let func = ||-> Result<pb::test::Boolean, Error>{
        
        let block: pb::test::Block = substreams::proto::decode_ptr(block_ptr, block_len).unwrap();
        let setup_test_store_min_float64: substreams::store::StoreGetFloat64 = substreams::store::StoreGetFloat64::new(setup_test_store_min_float64_ptr);
        let raw_setup_test_store_min_float64_deltas = substreams::proto::decode_ptr::<substreams::pb::substreams::StoreDeltas>(setup_test_store_min_float64_deltas_ptr, setup_test_store_min_float64_deltas_len).unwrap().deltas;
		let setup_test_store_min_float64_deltas: substreams::store::Deltas<substreams::store::DeltaFloat64> = substreams::store::Deltas::new(raw_setup_test_store_min_float64_deltas);

        Substreams::assert_test_store_min_float64_deltas(block,
            setup_test_store_min_float64,
            setup_test_store_min_float64_deltas,
            
        )
    };
    let result = func();
    if result.is_err() {
        panic!("{:?}", &result.err().unwrap());
    }
    substreams::output(result.unwrap());
}

#[no_mangle]
pub extern "C" fn setup_test_store_max_float64(
    block_ptr: *mut u8,
    block_len: usize,
) {
    substreams::register_panic_hook();
    let func = ||{
        
        let store: substreams::store::StoreMaxFloat64 = substreams::store::StoreMaxFloat64::new();
        
        let block: pb::test::Block = substreams::proto::decode_ptr(block_ptr, block_len).unwrap();

        Substreams::setup_test_store_max_float64(block,
            store,
        )
    };
    func()
}

#[no_mangle]
pub extern "C" fn assert_test_store_max_float64(
    block_ptr: *mut u8,
    block_len: usize,
    setup_test_store_max_float64_ptr: u32,
) {
    substreams::register_panic_hook();
    let func = ||-> Result<pb::test::Boolean, Error>{
        
        let block: pb::test::Block = substreams::proto::decode_ptr(block_ptr, block_len).unwrap();
        let setup_test_store_max_float64: substreams::store::StoreGetFloat64 = substreams::store::StoreGetFloat64::new(setup_test_store_max_float64_ptr);

        Substreams::assert_test_store_max_float64(block,
            setup_test_store_max_float64,
            
        )
    };
    let result = func();
    if result.is_err() {
        panic!("{:?}", &result.err().unwrap());
    }
    substreams::output(result.unwrap());
}

#[no_mangle]
pub extern "C" fn assert_test_store_max_float64_deltas(
    block_ptr: *mut u8,
    block_len: usize,
    setup_test_store_max_float64_ptr: u32,
    setup_test_store_max_float64_deltas_ptr: *mut u8,
    setup_test_store_max_float64_deltas_len: usize,
) {
    substreams::register_panic_hook();
    let func = ||-> Result<pb::test::Boolean, Error>{
        
        let block: pb::test::Block = substreams::proto::decode_ptr(block_ptr, block_len).unwrap();
        let setup_test_store_max_float64: substreams::store::StoreGetFloat64 = substreams::store::StoreGetFloat64::new(setup_test_store_max_float64_ptr);
        let raw_setup_test_store_max_float64_deltas = substreams::proto::decode_ptr::<substreams::pb::substreams::StoreDeltas>(setup_test_store_max_float64_deltas_ptr, setup_test_store_max_float64_deltas_len).unwrap().deltas;
		let setup_test_store_max_float64_deltas: substreams::store::Deltas<substreams::store::DeltaFloat64> = substreams::store::Deltas::new(raw_setup_test_store_max_float64_deltas);

        Substreams::assert_test_store_max_float64_deltas(block,
            setup_test_store_max_float64,
            setup_test_store_max_float64_deltas,
            
        )
    };
    let result = func();
    if result.is_err() {
        panic!("{:?}", &result.err().unwrap());
    }
    substreams::output(result.unwrap());
}

#[no_mangle]
pub extern "C" fn setup_test_store_add_bigint(
    block_ptr: *mut u8,
    block_len: usize,
) {
    substreams::register_panic_hook();
    let func = ||{
        
        let store: substreams::store::StoreAddBigInt = substreams::store::StoreAddBigInt::new();
        
        let block: pb::test::Block = substreams::proto::decode_ptr(block_ptr, block_len).unwrap();

        Substreams::setup_test_store_add_bigint(block,
            store,
        )
    };
    func()
}

#[no_mangle]
pub extern "C" fn assert_test_store_add_bigint(
    block_ptr: *mut u8,
    block_len: usize,
    setup_test_store_add_bigint_ptr: u32,
) {
    substreams::register_panic_hook();
    let func = ||-> Result<pb::test::Boolean, Error>{
        
        let block: pb::test::Block = substreams::proto::decode_ptr(block_ptr, block_len).unwrap();
        let setup_test_store_add_bigint: substreams::store::StoreGetBigInt = substreams::store::StoreGetBigInt::new(setup_test_store_add_bigint_ptr);

        Substreams::assert_test_store_add_bigint(block,
            setup_test_store_add_bigint,
            
        )
    };
    let result = func();
    if result.is_err() {
        panic!("{:?}", &result.err().unwrap());
    }
    substreams::output(result.unwrap());
}

#[no_mangle]
pub extern "C" fn assert_test_store_add_bigint_deltas(
    block_ptr: *mut u8,
    block_len: usize,
    setup_test_store_add_bigint_ptr: u32,
    setup_test_store_add_bigint_deltas_ptr: *mut u8,
    setup_test_store_add_bigint_deltas_len: usize,
) {
    substreams::register_panic_hook();
    let func = ||-> Result<pb::test::Boolean, Error>{
        
        let block: pb::test::Block = substreams::proto::decode_ptr(block_ptr, block_len).unwrap();
        let setup_test_store_add_bigint: substreams::store::StoreGetBigInt = substreams::store::StoreGetBigInt::new(setup_test_store_add_bigint_ptr);
        let raw_setup_test_store_add_bigint_deltas = substreams::proto::decode_ptr::<substreams::pb::substreams::StoreDeltas>(setup_test_store_add_bigint_deltas_ptr, setup_test_store_add_bigint_deltas_len).unwrap().deltas;
		let setup_test_store_add_bigint_deltas: substreams::store::Deltas<substreams::store::DeltaBigInt> = substreams::store::Deltas::new(raw_setup_test_store_add_bigint_deltas);

        Substreams::assert_test_store_add_bigint_deltas(block,
            setup_test_store_add_bigint,
            setup_test_store_add_bigint_deltas,
            
        )
    };
    let result = func();
    if result.is_err() {
        panic!("{:?}", &result.err().unwrap());
    }
    substreams::output(result.unwrap());
}

#[no_mangle]
pub extern "C" fn setup_test_store_set_bigint(
    block_ptr: *mut u8,
    block_len: usize,
) {
    substreams::register_panic_hook();
    let func = ||{
        
        let store: substreams::store::StoreSetBigInt = substreams::store::StoreSetBigInt::new();
        
        let block: pb::test::Block = substreams::proto::decode_ptr(block_ptr, block_len).unwrap();

        Substreams::setup_test_store_set_bigint(block,
            store,
        )
    };
    func()
}

#[no_mangle]
pub extern "C" fn assert_test_store_set_bigint(
    block_ptr: *mut u8,
    block_len: usize,
    setup_test_store_set_bigint_ptr: u32,
) {
    substreams::register_panic_hook();
    let func = ||-> Result<pb::test::Boolean, Error>{
        
        let block: pb::test::Block = substreams::proto::decode_ptr(block_ptr, block_len).unwrap();
        let setup_test_store_set_bigint: substreams::store::StoreGetBigInt = substreams::store::StoreGetBigInt::new(setup_test_store_set_bigint_ptr);

        Substreams::assert_test_store_set_bigint(block,
            setup_test_store_set_bigint,
            
        )
    };
    let result = func();
    if result.is_err() {
        panic!("{:?}", &result.err().unwrap());
    }
    substreams::output(result.unwrap());
}

#[no_mangle]
pub extern "C" fn assert_test_store_set_bigint_deltas(
    block_ptr: *mut u8,
    block_len: usize,
    setup_test_store_set_bigint_ptr: u32,
    setup_test_store_set_bigint_deltas_ptr: *mut u8,
    setup_test_store_set_bigint_deltas_len: usize,
) {
    substreams::register_panic_hook();
    let func = ||-> Result<pb::test::Boolean, Error>{
        
        let block: pb::test::Block = substreams::proto::decode_ptr(block_ptr, block_len).unwrap();
        let setup_test_store_set_bigint: substreams::store::StoreGetBigInt = substreams::store::StoreGetBigInt::new(setup_test_store_set_bigint_ptr);
        let raw_setup_test_store_set_bigint_deltas = substreams::proto::decode_ptr::<substreams::pb::substreams::StoreDeltas>(setup_test_store_set_bigint_deltas_ptr, setup_test_store_set_bigint_deltas_len).unwrap().deltas;
		let setup_test_store_set_bigint_deltas: substreams::store::Deltas<substreams::store::DeltaBigInt> = substreams::store::Deltas::new(raw_setup_test_store_set_bigint_deltas);

        Substreams::assert_test_store_set_bigint_deltas(block,
            setup_test_store_set_bigint,
            setup_test_store_set_bigint_deltas,
            
        )
    };
    let result = func();
    if result.is_err() {
        panic!("{:?}", &result.err().unwrap());
    }
    substreams::output(result.unwrap());
}

#[no_mangle]
pub extern "C" fn setup_test_store_set_if_not_exists_bigint(
    block_ptr: *mut u8,
    block_len: usize,
) {
    substreams::register_panic_hook();
    let func = ||{
        
        let store: substreams::store::StoreSetIfNotExistsBigInt = substreams::store::StoreSetIfNotExistsBigInt::new();
        
        let block: pb::test::Block = substreams::proto::decode_ptr(block_ptr, block_len).unwrap();

        Substreams::setup_test_store_set_if_not_exists_bigint(block,
            store,
        )
    };
    func()
}

#[no_mangle]
pub extern "C" fn assert_test_store_set_if_not_exists_bigint(
    block_ptr: *mut u8,
    block_len: usize,
    setup_test_store_set_if_not_exists_bigint_ptr: u32,
) {
    substreams::register_panic_hook();
    let func = ||-> Result<pb::test::Boolean, Error>{
        
        let block: pb::test::Block = substreams::proto::decode_ptr(block_ptr, block_len).unwrap();
        let setup_test_store_set_if_not_exists_bigint: substreams::store::StoreGetBigInt = substreams::store::StoreGetBigInt::new(setup_test_store_set_if_not_exists_bigint_ptr);

        Substreams::assert_test_store_set_if_not_exists_bigint(block,
            setup_test_store_set_if_not_exists_bigint,
            
        )
    };
    let result = func();
    if result.is_err() {
        panic!("{:?}", &result.err().unwrap());
    }
    substreams::output(result.unwrap());
}

#[no_mangle]
pub extern "C" fn assert_test_store_set_if_not_exists_bigint_deltas(
    block_ptr: *mut u8,
    block_len: usize,
    setup_test_store_set_if_not_exists_bigint_ptr: u32,
    setup_test_store_set_if_not_exists_bigint_deltas_ptr: *mut u8,
    setup_test_store_set_if_not_exists_bigint_deltas_len: usize,
) {
    substreams::register_panic_hook();
    let func = ||-> Result<pb::test::Boolean, Error>{
        
        let block: pb::test::Block = substreams::proto::decode_ptr(block_ptr, block_len).unwrap();
        let setup_test_store_set_if_not_exists_bigint: substreams::store::StoreGetBigInt = substreams::store::StoreGetBigInt::new(setup_test_store_set_if_not_exists_bigint_ptr);
        let raw_setup_test_store_set_if_not_exists_bigint_deltas = substreams::proto::decode_ptr::<substreams::pb::substreams::StoreDeltas>(setup_test_store_set_if_not_exists_bigint_deltas_ptr, setup_test_store_set_if_not_exists_bigint_deltas_len).unwrap().deltas;
		let setup_test_store_set_if_not_exists_bigint_deltas: substreams::store::Deltas<substreams::store::DeltaBigInt> = substreams::store::Deltas::new(raw_setup_test_store_set_if_not_exists_bigint_deltas);

        Substreams::assert_test_store_set_if_not_exists_bigint_deltas(block,
            setup_test_store_set_if_not_exists_bigint,
            setup_test_store_set_if_not_exists_bigint_deltas,
            
        )
    };
    let result = func();
    if result.is_err() {
        panic!("{:?}", &result.err().unwrap());
    }
    substreams::output(result.unwrap());
}

#[no_mangle]
pub extern "C" fn setup_test_store_min_bigint(
    block_ptr: *mut u8,
    block_len: usize,
) {
    substreams::register_panic_hook();
    let func = ||{
        
        let store: substreams::store::StoreMinBigInt = substreams::store::StoreMinBigInt::new();
        
        let block: pb::test::Block = substreams::proto::decode_ptr(block_ptr, block_len).unwrap();

        Substreams::setup_test_store_min_bigint(block,
            store,
        )
    };
    func()
}

#[no_mangle]
pub extern "C" fn assert_test_store_min_bigint(
    block_ptr: *mut u8,
    block_len: usize,
    setup_test_store_min_bigint_ptr: u32,
) {
    substreams::register_panic_hook();
    let func = ||-> Result<pb::test::Boolean, Error>{
        
        let block: pb::test::Block = substreams::proto::decode_ptr(block_ptr, block_len).unwrap();
        let setup_test_store_min_bigint: substreams::store::StoreGetBigInt = substreams::store::StoreGetBigInt::new(setup_test_store_min_bigint_ptr);

        Substreams::assert_test_store_min_bigint(block,
            setup_test_store_min_bigint,
            
        )
    };
    let result = func();
    if result.is_err() {
        panic!("{:?}", &result.err().unwrap());
    }
    substreams::output(result.unwrap());
}

#[no_mangle]
pub extern "C" fn assert_test_store_min_bigint_deltas(
    block_ptr: *mut u8,
    block_len: usize,
    setup_test_store_min_bigint_ptr: u32,
    setup_test_store_min_bigint_deltas_ptr: *mut u8,
    setup_test_store_min_bigint_deltas_len: usize,
) {
    substreams::register_panic_hook();
    let func = ||-> Result<pb::test::Boolean, Error>{
        
        let block: pb::test::Block = substreams::proto::decode_ptr(block_ptr, block_len).unwrap();
        let setup_test_store_min_bigint: substreams::store::StoreGetBigInt = substreams::store::StoreGetBigInt::new(setup_test_store_min_bigint_ptr);
        let raw_setup_test_store_min_bigint_deltas = substreams::proto::decode_ptr::<substreams::pb::substreams::StoreDeltas>(setup_test_store_min_bigint_deltas_ptr, setup_test_store_min_bigint_deltas_len).unwrap().deltas;
		let setup_test_store_min_bigint_deltas: substreams::store::Deltas<substreams::store::DeltaBigInt> = substreams::store::Deltas::new(raw_setup_test_store_min_bigint_deltas);

        Substreams::assert_test_store_min_bigint_deltas(block,
            setup_test_store_min_bigint,
            setup_test_store_min_bigint_deltas,
            
        )
    };
    let result = func();
    if result.is_err() {
        panic!("{:?}", &result.err().unwrap());
    }
    substreams::output(result.unwrap());
}

#[no_mangle]
pub extern "C" fn setup_test_store_max_bigint(
    block_ptr: *mut u8,
    block_len: usize,
) {
    substreams::register_panic_hook();
    let func = ||{
        
        let store: substreams::store::StoreMaxBigInt = substreams::store::StoreMaxBigInt::new();
        
        let block: pb::test::Block = substreams::proto::decode_ptr(block_ptr, block_len).unwrap();

        Substreams::setup_test_store_max_bigint(block,
            store,
        )
    };
    func()
}

#[no_mangle]
pub extern "C" fn assert_test_store_max_bigint(
    block_ptr: *mut u8,
    block_len: usize,
    setup_test_store_max_bigint_ptr: u32,
) {
    substreams::register_panic_hook();
    let func = ||-> Result<pb::test::Boolean, Error>{
        
        let block: pb::test::Block = substreams::proto::decode_ptr(block_ptr, block_len).unwrap();
        let setup_test_store_max_bigint: substreams::store::StoreGetBigInt = substreams::store::StoreGetBigInt::new(setup_test_store_max_bigint_ptr);

        Substreams::assert_test_store_max_bigint(block,
            setup_test_store_max_bigint,
            
        )
    };
    let result = func();
    if result.is_err() {
        panic!("{:?}", &result.err().unwrap());
    }
    substreams::output(result.unwrap());
}

#[no_mangle]
pub extern "C" fn assert_test_store_max_bigint_deltas(
    block_ptr: *mut u8,
    block_len: usize,
    setup_test_store_max_bigint_ptr: u32,
    setup_test_store_max_bigint_deltas_ptr: *mut u8,
    setup_test_store_max_bigint_deltas_len: usize,
) {
    substreams::register_panic_hook();
    let func = ||-> Result<pb::test::Boolean, Error>{
        
        let block: pb::test::Block = substreams::proto::decode_ptr(block_ptr, block_len).unwrap();
        let setup_test_store_max_bigint: substreams::store::StoreGetBigInt = substreams::store::StoreGetBigInt::new(setup_test_store_max_bigint_ptr);
        let raw_setup_test_store_max_bigint_deltas = substreams::proto::decode_ptr::<substreams::pb::substreams::StoreDeltas>(setup_test_store_max_bigint_deltas_ptr, setup_test_store_max_bigint_deltas_len).unwrap().deltas;
		let setup_test_store_max_bigint_deltas: substreams::store::Deltas<substreams::store::DeltaBigInt> = substreams::store::Deltas::new(raw_setup_test_store_max_bigint_deltas);

        Substreams::assert_test_store_max_bigint_deltas(block,
            setup_test_store_max_bigint,
            setup_test_store_max_bigint_deltas,
            
        )
    };
    let result = func();
    if result.is_err() {
        panic!("{:?}", &result.err().unwrap());
    }
    substreams::output(result.unwrap());
}

#[no_mangle]
pub extern "C" fn setup_test_store_add_bigdecimal(
    block_ptr: *mut u8,
    block_len: usize,
) {
    substreams::register_panic_hook();
    let func = ||{
        
        let store: substreams::store::StoreAddBigDecimal = substreams::store::StoreAddBigDecimal::new();
        
        let block: pb::test::Block = substreams::proto::decode_ptr(block_ptr, block_len).unwrap();

        Substreams::setup_test_store_add_bigdecimal(block,
            store,
        )
    };
    func()
}

#[no_mangle]
pub extern "C" fn assert_test_store_add_bigdecimal(
    block_ptr: *mut u8,
    block_len: usize,
    setup_test_store_add_bigdecimal_ptr: u32,
) {
    substreams::register_panic_hook();
    let func = ||-> Result<pb::test::Boolean, Error>{
        
        let block: pb::test::Block = substreams::proto::decode_ptr(block_ptr, block_len).unwrap();
        let setup_test_store_add_bigdecimal: substreams::store::StoreGetBigDecimal = substreams::store::StoreGetBigDecimal::new(setup_test_store_add_bigdecimal_ptr);

        Substreams::assert_test_store_add_bigdecimal(block,
            setup_test_store_add_bigdecimal,
            
        )
    };
    let result = func();
    if result.is_err() {
        panic!("{:?}", &result.err().unwrap());
    }
    substreams::output(result.unwrap());
}

#[no_mangle]
pub extern "C" fn assert_test_store_add_bigdecimal_deltas(
    block_ptr: *mut u8,
    block_len: usize,
    setup_test_store_add_bigdecimal_ptr: u32,
    setup_test_store_add_bigdecimal_deltas_ptr: *mut u8,
    setup_test_store_add_bigdecimal_deltas_len: usize,
) {
    substreams::register_panic_hook();
    let func = ||-> Result<pb::test::Boolean, Error>{
        
        let block: pb::test::Block = substreams::proto::decode_ptr(block_ptr, block_len).unwrap();
        let setup_test_store_add_bigdecimal: substreams::store::StoreGetBigDecimal = substreams::store::StoreGetBigDecimal::new(setup_test_store_add_bigdecimal_ptr);
        let raw_setup_test_store_add_bigdecimal_deltas = substreams::proto::decode_ptr::<substreams::pb::substreams::StoreDeltas>(setup_test_store_add_bigdecimal_deltas_ptr, setup_test_store_add_bigdecimal_deltas_len).unwrap().deltas;
		let setup_test_store_add_bigdecimal_deltas: substreams::store::Deltas<substreams::store::DeltaBigDecimal> = substreams::store::Deltas::new(raw_setup_test_store_add_bigdecimal_deltas);

        Substreams::assert_test_store_add_bigdecimal_deltas(block,
            setup_test_store_add_bigdecimal,
            setup_test_store_add_bigdecimal_deltas,
            
        )
    };
    let result = func();
    if result.is_err() {
        panic!("{:?}", &result.err().unwrap());
    }
    substreams::output(result.unwrap());
}

#[no_mangle]
pub extern "C" fn setup_test_store_set_bigdecimal(
    block_ptr: *mut u8,
    block_len: usize,
) {
    substreams::register_panic_hook();
    let func = ||{
        
        let store: substreams::store::StoreSetBigDecimal = substreams::store::StoreSetBigDecimal::new();
        
        let block: pb::test::Block = substreams::proto::decode_ptr(block_ptr, block_len).unwrap();

        Substreams::setup_test_store_set_bigdecimal(block,
            store,
        )
    };
    func()
}

#[no_mangle]
pub extern "C" fn assert_test_store_set_bigdecimal(
    block_ptr: *mut u8,
    block_len: usize,
    setup_test_store_set_bigdecimal_ptr: u32,
) {
    substreams::register_panic_hook();
    let func = ||-> Result<pb::test::Boolean, Error>{
        
        let block: pb::test::Block = substreams::proto::decode_ptr(block_ptr, block_len).unwrap();
        let setup_test_store_set_bigdecimal: substreams::store::StoreGetBigDecimal = substreams::store::StoreGetBigDecimal::new(setup_test_store_set_bigdecimal_ptr);

        Substreams::assert_test_store_set_bigdecimal(block,
            setup_test_store_set_bigdecimal,
            
        )
    };
    let result = func();
    if result.is_err() {
        panic!("{:?}", &result.err().unwrap());
    }
    substreams::output(result.unwrap());
}

#[no_mangle]
pub extern "C" fn assert_test_store_set_bigdecimal_deltas(
    block_ptr: *mut u8,
    block_len: usize,
    setup_test_store_set_bigdecimal_ptr: u32,
    setup_test_store_set_bigdecimal_deltas_ptr: *mut u8,
    setup_test_store_set_bigdecimal_deltas_len: usize,
) {
    substreams::register_panic_hook();
    let func = ||-> Result<pb::test::Boolean, Error>{
        
        let block: pb::test::Block = substreams::proto::decode_ptr(block_ptr, block_len).unwrap();
        let setup_test_store_set_bigdecimal: substreams::store::StoreGetBigDecimal = substreams::store::StoreGetBigDecimal::new(setup_test_store_set_bigdecimal_ptr);
        let raw_setup_test_store_set_bigdecimal_deltas = substreams::proto::decode_ptr::<substreams::pb::substreams::StoreDeltas>(setup_test_store_set_bigdecimal_deltas_ptr, setup_test_store_set_bigdecimal_deltas_len).unwrap().deltas;
		let setup_test_store_set_bigdecimal_deltas: substreams::store::Deltas<substreams::store::DeltaBigDecimal> = substreams::store::Deltas::new(raw_setup_test_store_set_bigdecimal_deltas);

        Substreams::assert_test_store_set_bigdecimal_deltas(block,
            setup_test_store_set_bigdecimal,
            setup_test_store_set_bigdecimal_deltas,
            
        )
    };
    let result = func();
    if result.is_err() {
        panic!("{:?}", &result.err().unwrap());
    }
    substreams::output(result.unwrap());
}

#[no_mangle]
pub extern "C" fn setup_test_store_set_if_not_exists_bigdecimal(
    block_ptr: *mut u8,
    block_len: usize,
) {
    substreams::register_panic_hook();
    let func = ||{
        
        let store: substreams::store::StoreSetIfNotExistsBigDecimal = substreams::store::StoreSetIfNotExistsBigDecimal::new();
        
        let block: pb::test::Block = substreams::proto::decode_ptr(block_ptr, block_len).unwrap();

        Substreams::setup_test_store_set_if_not_exists_bigdecimal(block,
            store,
        )
    };
    func()
}

#[no_mangle]
pub extern "C" fn assert_test_store_set_if_not_exists_bigdecimal(
    block_ptr: *mut u8,
    block_len: usize,
    setup_test_store_set_if_not_exists_bigdecimal_ptr: u32,
) {
    substreams::register_panic_hook();
    let func = ||-> Result<pb::test::Boolean, Error>{
        
        let block: pb::test::Block = substreams::proto::decode_ptr(block_ptr, block_len).unwrap();
        let setup_test_store_set_if_not_exists_bigdecimal: substreams::store::StoreGetBigDecimal = substreams::store::StoreGetBigDecimal::new(setup_test_store_set_if_not_exists_bigdecimal_ptr);

        Substreams::assert_test_store_set_if_not_exists_bigdecimal(block,
            setup_test_store_set_if_not_exists_bigdecimal,
            
        )
    };
    let result = func();
    if result.is_err() {
        panic!("{:?}", &result.err().unwrap());
    }
    substreams::output(result.unwrap());
}

#[no_mangle]
pub extern "C" fn assert_test_store_set_if_not_exists_bigdecimal_deltas(
    block_ptr: *mut u8,
    block_len: usize,
    setup_test_store_set_if_not_exists_bigdecimal_ptr: u32,
    setup_test_store_set_if_not_exists_bigdecimal_deltas_ptr: *mut u8,
    setup_test_store_set_if_not_exists_bigdecimal_deltas_len: usize,
) {
    substreams::register_panic_hook();
    let func = ||-> Result<pb::test::Boolean, Error>{
        
        let block: pb::test::Block = substreams::proto::decode_ptr(block_ptr, block_len).unwrap();
        let setup_test_store_set_if_not_exists_bigdecimal: substreams::store::StoreGetBigDecimal = substreams::store::StoreGetBigDecimal::new(setup_test_store_set_if_not_exists_bigdecimal_ptr);
        let raw_setup_test_store_set_if_not_exists_bigdecimal_deltas = substreams::proto::decode_ptr::<substreams::pb::substreams::StoreDeltas>(setup_test_store_set_if_not_exists_bigdecimal_deltas_ptr, setup_test_store_set_if_not_exists_bigdecimal_deltas_len).unwrap().deltas;
		let setup_test_store_set_if_not_exists_bigdecimal_deltas: substreams::store::Deltas<substreams::store::DeltaBigDecimal> = substreams::store::Deltas::new(raw_setup_test_store_set_if_not_exists_bigdecimal_deltas);

        Substreams::assert_test_store_set_if_not_exists_bigdecimal_deltas(block,
            setup_test_store_set_if_not_exists_bigdecimal,
            setup_test_store_set_if_not_exists_bigdecimal_deltas,
            
        )
    };
    let result = func();
    if result.is_err() {
        panic!("{:?}", &result.err().unwrap());
    }
    substreams::output(result.unwrap());
}

#[no_mangle]
pub extern "C" fn setup_test_store_min_bigdecimal(
    block_ptr: *mut u8,
    block_len: usize,
) {
    substreams::register_panic_hook();
    let func = ||{
        
        let store: substreams::store::StoreMinBigDecimal = substreams::store::StoreMinBigDecimal::new();
        
        let block: pb::test::Block = substreams::proto::decode_ptr(block_ptr, block_len).unwrap();

        Substreams::setup_test_store_min_bigdecimal(block,
            store,
        )
    };
    func()
}

#[no_mangle]
pub extern "C" fn assert_test_store_min_bigdecimal(
    block_ptr: *mut u8,
    block_len: usize,
    setup_test_store_min_bigdecimal_ptr: u32,
) {
    substreams::register_panic_hook();
    let func = ||-> Result<pb::test::Boolean, Error>{
        
        let block: pb::test::Block = substreams::proto::decode_ptr(block_ptr, block_len).unwrap();
        let setup_test_store_min_bigdecimal: substreams::store::StoreGetBigDecimal = substreams::store::StoreGetBigDecimal::new(setup_test_store_min_bigdecimal_ptr);

        Substreams::assert_test_store_min_bigdecimal(block,
            setup_test_store_min_bigdecimal,
            
        )
    };
    let result = func();
    if result.is_err() {
        panic!("{:?}", &result.err().unwrap());
    }
    substreams::output(result.unwrap());
}

#[no_mangle]
pub extern "C" fn assert_test_store_min_bigdecimal_deltas(
    block_ptr: *mut u8,
    block_len: usize,
    setup_test_store_min_bigdecimal_ptr: u32,
    setup_test_store_min_bigdecimal_deltas_ptr: *mut u8,
    setup_test_store_min_bigdecimal_deltas_len: usize,
) {
    substreams::register_panic_hook();
    let func = ||-> Result<pb::test::Boolean, Error>{
        
        let block: pb::test::Block = substreams::proto::decode_ptr(block_ptr, block_len).unwrap();
        let setup_test_store_min_bigdecimal: substreams::store::StoreGetBigDecimal = substreams::store::StoreGetBigDecimal::new(setup_test_store_min_bigdecimal_ptr);
        let raw_setup_test_store_min_bigdecimal_deltas = substreams::proto::decode_ptr::<substreams::pb::substreams::StoreDeltas>(setup_test_store_min_bigdecimal_deltas_ptr, setup_test_store_min_bigdecimal_deltas_len).unwrap().deltas;
		let setup_test_store_min_bigdecimal_deltas: substreams::store::Deltas<substreams::store::DeltaBigDecimal> = substreams::store::Deltas::new(raw_setup_test_store_min_bigdecimal_deltas);

        Substreams::assert_test_store_min_bigdecimal_deltas(block,
            setup_test_store_min_bigdecimal,
            setup_test_store_min_bigdecimal_deltas,
            
        )
    };
    let result = func();
    if result.is_err() {
        panic!("{:?}", &result.err().unwrap());
    }
    substreams::output(result.unwrap());
}

#[no_mangle]
pub extern "C" fn setup_test_store_max_bigdecimal(
    block_ptr: *mut u8,
    block_len: usize,
) {
    substreams::register_panic_hook();
    let func = ||{
        
        let store: substreams::store::StoreMaxBigDecimal = substreams::store::StoreMaxBigDecimal::new();
        
        let block: pb::test::Block = substreams::proto::decode_ptr(block_ptr, block_len).unwrap();

        Substreams::setup_test_store_max_bigdecimal(block,
            store,
        )
    };
    func()
}

#[no_mangle]
pub extern "C" fn assert_test_store_max_bigdecimal(
    block_ptr: *mut u8,
    block_len: usize,
    setup_test_store_max_bigdecimal_ptr: u32,
) {
    substreams::register_panic_hook();
    let func = ||-> Result<pb::test::Boolean, Error>{
        
        let block: pb::test::Block = substreams::proto::decode_ptr(block_ptr, block_len).unwrap();
        let setup_test_store_max_bigdecimal: substreams::store::StoreGetBigDecimal = substreams::store::StoreGetBigDecimal::new(setup_test_store_max_bigdecimal_ptr);

        Substreams::assert_test_store_max_bigdecimal(block,
            setup_test_store_max_bigdecimal,
            
        )
    };
    let result = func();
    if result.is_err() {
        panic!("{:?}", &result.err().unwrap());
    }
    substreams::output(result.unwrap());
}

#[no_mangle]
pub extern "C" fn assert_test_store_max_bigdecimal_deltas(
    block_ptr: *mut u8,
    block_len: usize,
    setup_test_store_max_bigdecimal_ptr: u32,
    setup_test_store_max_bigdecimal_deltas_ptr: *mut u8,
    setup_test_store_max_bigdecimal_deltas_len: usize,
) {
    substreams::register_panic_hook();
    let func = ||-> Result<pb::test::Boolean, Error>{
        
        let block: pb::test::Block = substreams::proto::decode_ptr(block_ptr, block_len).unwrap();
        let setup_test_store_max_bigdecimal: substreams::store::StoreGetBigDecimal = substreams::store::StoreGetBigDecimal::new(setup_test_store_max_bigdecimal_ptr);
        let raw_setup_test_store_max_bigdecimal_deltas = substreams::proto::decode_ptr::<substreams::pb::substreams::StoreDeltas>(setup_test_store_max_bigdecimal_deltas_ptr, setup_test_store_max_bigdecimal_deltas_len).unwrap().deltas;
		let setup_test_store_max_bigdecimal_deltas: substreams::store::Deltas<substreams::store::DeltaBigDecimal> = substreams::store::Deltas::new(raw_setup_test_store_max_bigdecimal_deltas);

        Substreams::assert_test_store_max_bigdecimal_deltas(block,
            setup_test_store_max_bigdecimal,
            setup_test_store_max_bigdecimal_deltas,
            
        )
    };
    let result = func();
    if result.is_err() {
        panic!("{:?}", &result.err().unwrap());
    }
    substreams::output(result.unwrap());
}

#[no_mangle]
pub extern "C" fn setup_test_store_set_string(
    block_ptr: *mut u8,
    block_len: usize,
) {
    substreams::register_panic_hook();
    let func = ||{
        
        let store: substreams::store::StoreSetString = substreams::store::StoreSetString::new();
        
        let block: pb::test::Block = substreams::proto::decode_ptr(block_ptr, block_len).unwrap();

        Substreams::setup_test_store_set_string(block,
            store,
        )
    };
    func()
}

#[no_mangle]
pub extern "C" fn assert_test_store_set_string(
    block_ptr: *mut u8,
    block_len: usize,
    setup_test_store_set_string_ptr: u32,
) {
    substreams::register_panic_hook();
    let func = ||-> Result<pb::test::Boolean, Error>{
        
        let block: pb::test::Block = substreams::proto::decode_ptr(block_ptr, block_len).unwrap();
        let setup_test_store_set_string: substreams::store::StoreGetString = substreams::store::StoreGetString::new(setup_test_store_set_string_ptr);

        Substreams::assert_test_store_set_string(block,
            setup_test_store_set_string,
            
        )
    };
    let result = func();
    if result.is_err() {
        panic!("{:?}", &result.err().unwrap());
    }
    substreams::output(result.unwrap());
}

#[no_mangle]
pub extern "C" fn assert_test_store_set_string_deltas(
    block_ptr: *mut u8,
    block_len: usize,
    setup_test_store_set_string_ptr: u32,
    setup_test_store_set_string_deltas_ptr: *mut u8,
    setup_test_store_set_string_deltas_len: usize,
) {
    substreams::register_panic_hook();
    let func = ||-> Result<pb::test::Boolean, Error>{
        
        let block: pb::test::Block = substreams::proto::decode_ptr(block_ptr, block_len).unwrap();
        let setup_test_store_set_string: substreams::store::StoreGetString = substreams::store::StoreGetString::new(setup_test_store_set_string_ptr);
        let raw_setup_test_store_set_string_deltas = substreams::proto::decode_ptr::<substreams::pb::substreams::StoreDeltas>(setup_test_store_set_string_deltas_ptr, setup_test_store_set_string_deltas_len).unwrap().deltas;
		let setup_test_store_set_string_deltas: substreams::store::Deltas<substreams::store::DeltaString> = substreams::store::Deltas::new(raw_setup_test_store_set_string_deltas);

        Substreams::assert_test_store_set_string_deltas(block,
            setup_test_store_set_string,
            setup_test_store_set_string_deltas,
            
        )
    };
    let result = func();
    if result.is_err() {
        panic!("{:?}", &result.err().unwrap());
    }
    substreams::output(result.unwrap());
}

#[no_mangle]
pub extern "C" fn setup_test_store_set_if_not_exists_string(
    block_ptr: *mut u8,
    block_len: usize,
) {
    substreams::register_panic_hook();
    let func = ||{
        
        let store: substreams::store::StoreSetIfNotExistsString = substreams::store::StoreSetIfNotExistsString::new();
        
        let block: pb::test::Block = substreams::proto::decode_ptr(block_ptr, block_len).unwrap();

        Substreams::setup_test_store_set_if_not_exists_string(block,
            store,
        )
    };
    func()
}

#[no_mangle]
pub extern "C" fn assert_test_store_set_if_not_exists_string(
    block_ptr: *mut u8,
    block_len: usize,
    setup_test_store_set_if_not_exists_string_ptr: u32,
) {
    substreams::register_panic_hook();
    let func = ||-> Result<pb::test::Boolean, Error>{
        
        let block: pb::test::Block = substreams::proto::decode_ptr(block_ptr, block_len).unwrap();
        let setup_test_store_set_if_not_exists_string: substreams::store::StoreGetString = substreams::store::StoreGetString::new(setup_test_store_set_if_not_exists_string_ptr);

        Substreams::assert_test_store_set_if_not_exists_string(block,
            setup_test_store_set_if_not_exists_string,
            
        )
    };
    let result = func();
    if result.is_err() {
        panic!("{:?}", &result.err().unwrap());
    }
    substreams::output(result.unwrap());
}

#[no_mangle]
pub extern "C" fn assert_test_store_set_if_not_exists_string_deltas(
    block_ptr: *mut u8,
    block_len: usize,
    setup_test_store_set_if_not_exists_string_ptr: u32,
    setup_test_store_set_if_not_exists_string_deltas_ptr: *mut u8,
    setup_test_store_set_if_not_exists_string_deltas_len: usize,
) {
    substreams::register_panic_hook();
    let func = ||-> Result<pb::test::Boolean, Error>{
        
        let block: pb::test::Block = substreams::proto::decode_ptr(block_ptr, block_len).unwrap();
        let setup_test_store_set_if_not_exists_string: substreams::store::StoreGetString = substreams::store::StoreGetString::new(setup_test_store_set_if_not_exists_string_ptr);
        let raw_setup_test_store_set_if_not_exists_string_deltas = substreams::proto::decode_ptr::<substreams::pb::substreams::StoreDeltas>(setup_test_store_set_if_not_exists_string_deltas_ptr, setup_test_store_set_if_not_exists_string_deltas_len).unwrap().deltas;
		let setup_test_store_set_if_not_exists_string_deltas: substreams::store::Deltas<substreams::store::DeltaString> = substreams::store::Deltas::new(raw_setup_test_store_set_if_not_exists_string_deltas);

        Substreams::assert_test_store_set_if_not_exists_string_deltas(block,
            setup_test_store_set_if_not_exists_string,
            setup_test_store_set_if_not_exists_string_deltas,
            
        )
    };
    let result = func();
    if result.is_err() {
        panic!("{:?}", &result.err().unwrap());
    }
    substreams::output(result.unwrap());
}

#[no_mangle]
pub extern "C" fn setup_test_store_append_string(
    block_ptr: *mut u8,
    block_len: usize,
) {
    substreams::register_panic_hook();
    let func = ||{
        
        let store: substreams::store::StoreAppend<String> = substreams::store::StoreAppend::new();
        
        let block: pb::test::Block = substreams::proto::decode_ptr(block_ptr, block_len).unwrap();

        Substreams::setup_test_store_append_string(block,
            store,
        )
    };
    func()
}

#[no_mangle]
pub extern "C" fn assert_test_store_append_string(
    block_ptr: *mut u8,
    block_len: usize,
    setup_test_store_append_string_ptr: u32,
) {
    substreams::register_panic_hook();
    let func = ||-> Result<pb::test::Boolean, Error>{
        
        let block: pb::test::Block = substreams::proto::decode_ptr(block_ptr, block_len).unwrap();
        let setup_test_store_append_string: substreams::store::StoreGetRaw = substreams::store::StoreGetRaw::new(setup_test_store_append_string_ptr);

        Substreams::assert_test_store_append_string(block,
            setup_test_store_append_string,
            
        )
    };
    let result = func();
    if result.is_err() {
        panic!("{:?}", &result.err().unwrap());
    }
    substreams::output(result.unwrap());
}

#[no_mangle]
pub extern "C" fn assert_test_store_append_string_deltas(
    block_ptr: *mut u8,
    block_len: usize,
    setup_test_store_append_string_ptr: u32,
    setup_test_store_append_string_deltas_ptr: *mut u8,
    setup_test_store_append_string_deltas_len: usize,
) {
    substreams::register_panic_hook();
    let func = ||-> Result<pb::test::Boolean, Error>{
        
        let block: pb::test::Block = substreams::proto::decode_ptr(block_ptr, block_len).unwrap();
        let setup_test_store_append_string: substreams::store::StoreGetRaw = substreams::store::StoreGetRaw::new(setup_test_store_append_string_ptr);
        let raw_setup_test_store_append_string_deltas = substreams::proto::decode_ptr::<substreams::pb::substreams::StoreDeltas>(setup_test_store_append_string_deltas_ptr, setup_test_store_append_string_deltas_len).unwrap().deltas;
		let setup_test_store_append_string_deltas: substreams::store::Deltas<substreams::store::DeltaArray<String>> = substreams::store::Deltas::new(raw_setup_test_store_append_string_deltas);

        Substreams::assert_test_store_append_string_deltas(block,
            setup_test_store_append_string,
            setup_test_store_append_string_deltas,
            
        )
    };
    let result = func();
    if result.is_err() {
        panic!("{:?}", &result.err().unwrap());
    }
    substreams::output(result.unwrap());
}

#[no_mangle]
pub extern "C" fn store_root(
    block_ptr: *mut u8,
    block_len: usize,
) {
    substreams::register_panic_hook();
    let func = ||{
        
        let store: substreams::store::StoreSetInt64 = substreams::store::StoreSetInt64::new();
        
        let block: pb::test::Block = substreams::proto::decode_ptr(block_ptr, block_len).unwrap();

        Substreams::store_root(block,
            store,
        )
    };
    func()
}

#[no_mangle]
pub extern "C" fn store_depend(
    block_ptr: *mut u8,
    block_len: usize,
    store_root_ptr: u32,
) {
    substreams::register_panic_hook();
    let func = ||{
        
        let store: substreams::store::StoreSetInt64 = substreams::store::StoreSetInt64::new();
        
        let block: pb::test::Block = substreams::proto::decode_ptr(block_ptr, block_len).unwrap();
        let store_root: substreams::store::StoreGetInt64 = substreams::store::StoreGetInt64::new(store_root_ptr);

        Substreams::store_depend(block,
            store_root,
            store,
        )
    };
    func()
}

#[no_mangle]
pub extern "C" fn store_depends_on_depend(
    block_ptr: *mut u8,
    block_len: usize,
    store_root_ptr: u32,
    store_depend_ptr: u32,
) {
    substreams::register_panic_hook();
    let func = ||{
        
        let store: substreams::store::StoreSetInt64 = substreams::store::StoreSetInt64::new();
        
        let block: pb::test::Block = substreams::proto::decode_ptr(block_ptr, block_len).unwrap();
        let store_root: substreams::store::StoreGetInt64 = substreams::store::StoreGetInt64::new(store_root_ptr);
        let store_depend: substreams::store::StoreGetInt64 = substreams::store::StoreGetInt64::new(store_depend_ptr);

        Substreams::store_depends_on_depend(block,
            store_root,
            store_depend,
            store,
        )
    };
    func()
}

#[no_mangle]
pub extern "C" fn assert_all_test_i64(
    assert_test_store_add_i64_ptr: *mut u8,
    assert_test_store_add_i64_len: usize,
    assert_test_store_add_i64_deltas_ptr: *mut u8,
    assert_test_store_add_i64_deltas_len: usize,
    assert_test_store_set_i64_ptr: *mut u8,
    assert_test_store_set_i64_len: usize,
    assert_test_store_set_i64_deltas_ptr: *mut u8,
    assert_test_store_set_i64_deltas_len: usize,
    assert_test_store_set_if_not_exists_i64_ptr: *mut u8,
    assert_test_store_set_if_not_exists_i64_len: usize,
    assert_test_store_set_if_not_exists_i64_deltas_ptr: *mut u8,
    assert_test_store_set_if_not_exists_i64_deltas_len: usize,
    assert_test_store_min_i64_ptr: *mut u8,
    assert_test_store_min_i64_len: usize,
    assert_test_store_min_i64_deltas_ptr: *mut u8,
    assert_test_store_min_i64_deltas_len: usize,
    assert_test_store_max_i64_ptr: *mut u8,
    assert_test_store_max_i64_len: usize,
    assert_test_store_max_i64_deltas_ptr: *mut u8,
    assert_test_store_max_i64_deltas_len: usize,
) {
    substreams::register_panic_hook();
    let func = ||{
        
        let store: substreams::store::StoreSetInt64 = substreams::store::StoreSetInt64::new();
        
        let assert_test_store_add_i64: pb::test::Boolean = substreams::proto::decode_ptr(assert_test_store_add_i64_ptr, assert_test_store_add_i64_len).unwrap();
        let assert_test_store_add_i64_deltas: pb::test::Boolean = substreams::proto::decode_ptr(assert_test_store_add_i64_deltas_ptr, assert_test_store_add_i64_deltas_len).unwrap();
        let assert_test_store_set_i64: pb::test::Boolean = substreams::proto::decode_ptr(assert_test_store_set_i64_ptr, assert_test_store_set_i64_len).unwrap();
        let assert_test_store_set_i64_deltas: pb::test::Boolean = substreams::proto::decode_ptr(assert_test_store_set_i64_deltas_ptr, assert_test_store_set_i64_deltas_len).unwrap();
        let assert_test_store_set_if_not_exists_i64: pb::test::Boolean = substreams::proto::decode_ptr(assert_test_store_set_if_not_exists_i64_ptr, assert_test_store_set_if_not_exists_i64_len).unwrap();
        let assert_test_store_set_if_not_exists_i64_deltas: pb::test::Boolean = substreams::proto::decode_ptr(assert_test_store_set_if_not_exists_i64_deltas_ptr, assert_test_store_set_if_not_exists_i64_deltas_len).unwrap();
        let assert_test_store_min_i64: pb::test::Boolean = substreams::proto::decode_ptr(assert_test_store_min_i64_ptr, assert_test_store_min_i64_len).unwrap();
        let assert_test_store_min_i64_deltas: pb::test::Boolean = substreams::proto::decode_ptr(assert_test_store_min_i64_deltas_ptr, assert_test_store_min_i64_deltas_len).unwrap();
        let assert_test_store_max_i64: pb::test::Boolean = substreams::proto::decode_ptr(assert_test_store_max_i64_ptr, assert_test_store_max_i64_len).unwrap();
        let assert_test_store_max_i64_deltas: pb::test::Boolean = substreams::proto::decode_ptr(assert_test_store_max_i64_deltas_ptr, assert_test_store_max_i64_deltas_len).unwrap();

        Substreams::assert_all_test_i64(assert_test_store_add_i64,
            assert_test_store_add_i64_deltas,
            assert_test_store_set_i64,
            assert_test_store_set_i64_deltas,
            assert_test_store_set_if_not_exists_i64,
            assert_test_store_set_if_not_exists_i64_deltas,
            assert_test_store_min_i64,
            assert_test_store_min_i64_deltas,
            assert_test_store_max_i64,
            assert_test_store_max_i64_deltas,
            store,
        )
    };
    func()
}

#[no_mangle]
pub extern "C" fn assert_all_test_float64(
    assert_test_store_add_float64_ptr: *mut u8,
    assert_test_store_add_float64_len: usize,
    assert_test_store_add_float64_deltas_ptr: *mut u8,
    assert_test_store_add_float64_deltas_len: usize,
    assert_test_store_set_float64_ptr: *mut u8,
    assert_test_store_set_float64_len: usize,
    assert_test_store_set_float64_deltas_ptr: *mut u8,
    assert_test_store_set_float64_deltas_len: usize,
    assert_test_store_set_if_not_exists_float64_ptr: *mut u8,
    assert_test_store_set_if_not_exists_float64_len: usize,
    assert_test_store_set_if_not_exists_float64_deltas_ptr: *mut u8,
    assert_test_store_set_if_not_exists_float64_deltas_len: usize,
    assert_test_store_min_float64_ptr: *mut u8,
    assert_test_store_min_float64_len: usize,
    assert_test_store_min_float64_deltas_ptr: *mut u8,
    assert_test_store_min_float64_deltas_len: usize,
    assert_test_store_max_float64_ptr: *mut u8,
    assert_test_store_max_float64_len: usize,
    assert_test_store_max_float64_deltas_ptr: *mut u8,
    assert_test_store_max_float64_deltas_len: usize,
) {
    substreams::register_panic_hook();
    let func = ||{
        
        let store: substreams::store::StoreSetInt64 = substreams::store::StoreSetInt64::new();
        
        let assert_test_store_add_float64: pb::test::Boolean = substreams::proto::decode_ptr(assert_test_store_add_float64_ptr, assert_test_store_add_float64_len).unwrap();
        let assert_test_store_add_float64_deltas: pb::test::Boolean = substreams::proto::decode_ptr(assert_test_store_add_float64_deltas_ptr, assert_test_store_add_float64_deltas_len).unwrap();
        let assert_test_store_set_float64: pb::test::Boolean = substreams::proto::decode_ptr(assert_test_store_set_float64_ptr, assert_test_store_set_float64_len).unwrap();
        let assert_test_store_set_float64_deltas: pb::test::Boolean = substreams::proto::decode_ptr(assert_test_store_set_float64_deltas_ptr, assert_test_store_set_float64_deltas_len).unwrap();
        let assert_test_store_set_if_not_exists_float64: pb::test::Boolean = substreams::proto::decode_ptr(assert_test_store_set_if_not_exists_float64_ptr, assert_test_store_set_if_not_exists_float64_len).unwrap();
        let assert_test_store_set_if_not_exists_float64_deltas: pb::test::Boolean = substreams::proto::decode_ptr(assert_test_store_set_if_not_exists_float64_deltas_ptr, assert_test_store_set_if_not_exists_float64_deltas_len).unwrap();
        let assert_test_store_min_float64: pb::test::Boolean = substreams::proto::decode_ptr(assert_test_store_min_float64_ptr, assert_test_store_min_float64_len).unwrap();
        let assert_test_store_min_float64_deltas: pb::test::Boolean = substreams::proto::decode_ptr(assert_test_store_min_float64_deltas_ptr, assert_test_store_min_float64_deltas_len).unwrap();
        let assert_test_store_max_float64: pb::test::Boolean = substreams::proto::decode_ptr(assert_test_store_max_float64_ptr, assert_test_store_max_float64_len).unwrap();
        let assert_test_store_max_float64_deltas: pb::test::Boolean = substreams::proto::decode_ptr(assert_test_store_max_float64_deltas_ptr, assert_test_store_max_float64_deltas_len).unwrap();

        Substreams::assert_all_test_float64(assert_test_store_add_float64,
            assert_test_store_add_float64_deltas,
            assert_test_store_set_float64,
            assert_test_store_set_float64_deltas,
            assert_test_store_set_if_not_exists_float64,
            assert_test_store_set_if_not_exists_float64_deltas,
            assert_test_store_min_float64,
            assert_test_store_min_float64_deltas,
            assert_test_store_max_float64,
            assert_test_store_max_float64_deltas,
            store,
        )
    };
    func()
}

#[no_mangle]
pub extern "C" fn assert_all_test_bigint(
    assert_test_store_add_bigint_ptr: *mut u8,
    assert_test_store_add_bigint_len: usize,
    assert_test_store_add_bigint_deltas_ptr: *mut u8,
    assert_test_store_add_bigint_deltas_len: usize,
    assert_test_store_set_bigint_ptr: *mut u8,
    assert_test_store_set_bigint_len: usize,
    assert_test_store_set_bigint_deltas_ptr: *mut u8,
    assert_test_store_set_bigint_deltas_len: usize,
    assert_test_store_set_if_not_exists_bigint_ptr: *mut u8,
    assert_test_store_set_if_not_exists_bigint_len: usize,
    assert_test_store_set_if_not_exists_bigint_deltas_ptr: *mut u8,
    assert_test_store_set_if_not_exists_bigint_deltas_len: usize,
    assert_test_store_min_bigint_ptr: *mut u8,
    assert_test_store_min_bigint_len: usize,
    assert_test_store_min_bigint_deltas_ptr: *mut u8,
    assert_test_store_min_bigint_deltas_len: usize,
    assert_test_store_max_bigint_ptr: *mut u8,
    assert_test_store_max_bigint_len: usize,
    assert_test_store_max_bigint_deltas_ptr: *mut u8,
    assert_test_store_max_bigint_deltas_len: usize,
) {
    substreams::register_panic_hook();
    let func = ||{
        
        let store: substreams::store::StoreSetInt64 = substreams::store::StoreSetInt64::new();
        
        let assert_test_store_add_bigint: pb::test::Boolean = substreams::proto::decode_ptr(assert_test_store_add_bigint_ptr, assert_test_store_add_bigint_len).unwrap();
        let assert_test_store_add_bigint_deltas: pb::test::Boolean = substreams::proto::decode_ptr(assert_test_store_add_bigint_deltas_ptr, assert_test_store_add_bigint_deltas_len).unwrap();
        let assert_test_store_set_bigint: pb::test::Boolean = substreams::proto::decode_ptr(assert_test_store_set_bigint_ptr, assert_test_store_set_bigint_len).unwrap();
        let assert_test_store_set_bigint_deltas: pb::test::Boolean = substreams::proto::decode_ptr(assert_test_store_set_bigint_deltas_ptr, assert_test_store_set_bigint_deltas_len).unwrap();
        let assert_test_store_set_if_not_exists_bigint: pb::test::Boolean = substreams::proto::decode_ptr(assert_test_store_set_if_not_exists_bigint_ptr, assert_test_store_set_if_not_exists_bigint_len).unwrap();
        let assert_test_store_set_if_not_exists_bigint_deltas: pb::test::Boolean = substreams::proto::decode_ptr(assert_test_store_set_if_not_exists_bigint_deltas_ptr, assert_test_store_set_if_not_exists_bigint_deltas_len).unwrap();
        let assert_test_store_min_bigint: pb::test::Boolean = substreams::proto::decode_ptr(assert_test_store_min_bigint_ptr, assert_test_store_min_bigint_len).unwrap();
        let assert_test_store_min_bigint_deltas: pb::test::Boolean = substreams::proto::decode_ptr(assert_test_store_min_bigint_deltas_ptr, assert_test_store_min_bigint_deltas_len).unwrap();
        let assert_test_store_max_bigint: pb::test::Boolean = substreams::proto::decode_ptr(assert_test_store_max_bigint_ptr, assert_test_store_max_bigint_len).unwrap();
        let assert_test_store_max_bigint_deltas: pb::test::Boolean = substreams::proto::decode_ptr(assert_test_store_max_bigint_deltas_ptr, assert_test_store_max_bigint_deltas_len).unwrap();

        Substreams::assert_all_test_bigint(assert_test_store_add_bigint,
            assert_test_store_add_bigint_deltas,
            assert_test_store_set_bigint,
            assert_test_store_set_bigint_deltas,
            assert_test_store_set_if_not_exists_bigint,
            assert_test_store_set_if_not_exists_bigint_deltas,
            assert_test_store_min_bigint,
            assert_test_store_min_bigint_deltas,
            assert_test_store_max_bigint,
            assert_test_store_max_bigint_deltas,
            store,
        )
    };
    func()
}

#[no_mangle]
pub extern "C" fn assert_all_test_bigdecimal(
    assert_test_store_add_bigdecimal_ptr: *mut u8,
    assert_test_store_add_bigdecimal_len: usize,
    assert_test_store_add_bigdecimal_deltas_ptr: *mut u8,
    assert_test_store_add_bigdecimal_deltas_len: usize,
    assert_test_store_set_bigdecimal_ptr: *mut u8,
    assert_test_store_set_bigdecimal_len: usize,
    assert_test_store_set_bigdecimal_deltas_ptr: *mut u8,
    assert_test_store_set_bigdecimal_deltas_len: usize,
    assert_test_store_set_if_not_exists_bigdecimal_ptr: *mut u8,
    assert_test_store_set_if_not_exists_bigdecimal_len: usize,
    assert_test_store_set_if_not_exists_bigdecimal_deltas_ptr: *mut u8,
    assert_test_store_set_if_not_exists_bigdecimal_deltas_len: usize,
    assert_test_store_min_bigdecimal_ptr: *mut u8,
    assert_test_store_min_bigdecimal_len: usize,
    assert_test_store_min_bigdecimal_deltas_ptr: *mut u8,
    assert_test_store_min_bigdecimal_deltas_len: usize,
    assert_test_store_max_bigdecimal_ptr: *mut u8,
    assert_test_store_max_bigdecimal_len: usize,
    assert_test_store_max_bigdecimal_deltas_ptr: *mut u8,
    assert_test_store_max_bigdecimal_deltas_len: usize,
) {
    substreams::register_panic_hook();
    let func = ||{
        
        let store: substreams::store::StoreSetInt64 = substreams::store::StoreSetInt64::new();
        
        let assert_test_store_add_bigdecimal: pb::test::Boolean = substreams::proto::decode_ptr(assert_test_store_add_bigdecimal_ptr, assert_test_store_add_bigdecimal_len).unwrap();
        let assert_test_store_add_bigdecimal_deltas: pb::test::Boolean = substreams::proto::decode_ptr(assert_test_store_add_bigdecimal_deltas_ptr, assert_test_store_add_bigdecimal_deltas_len).unwrap();
        let assert_test_store_set_bigdecimal: pb::test::Boolean = substreams::proto::decode_ptr(assert_test_store_set_bigdecimal_ptr, assert_test_store_set_bigdecimal_len).unwrap();
        let assert_test_store_set_bigdecimal_deltas: pb::test::Boolean = substreams::proto::decode_ptr(assert_test_store_set_bigdecimal_deltas_ptr, assert_test_store_set_bigdecimal_deltas_len).unwrap();
        let assert_test_store_set_if_not_exists_bigdecimal: pb::test::Boolean = substreams::proto::decode_ptr(assert_test_store_set_if_not_exists_bigdecimal_ptr, assert_test_store_set_if_not_exists_bigdecimal_len).unwrap();
        let assert_test_store_set_if_not_exists_bigdecimal_deltas: pb::test::Boolean = substreams::proto::decode_ptr(assert_test_store_set_if_not_exists_bigdecimal_deltas_ptr, assert_test_store_set_if_not_exists_bigdecimal_deltas_len).unwrap();
        let assert_test_store_min_bigdecimal: pb::test::Boolean = substreams::proto::decode_ptr(assert_test_store_min_bigdecimal_ptr, assert_test_store_min_bigdecimal_len).unwrap();
        let assert_test_store_min_bigdecimal_deltas: pb::test::Boolean = substreams::proto::decode_ptr(assert_test_store_min_bigdecimal_deltas_ptr, assert_test_store_min_bigdecimal_deltas_len).unwrap();
        let assert_test_store_max_bigdecimal: pb::test::Boolean = substreams::proto::decode_ptr(assert_test_store_max_bigdecimal_ptr, assert_test_store_max_bigdecimal_len).unwrap();
        let assert_test_store_max_bigdecimal_deltas: pb::test::Boolean = substreams::proto::decode_ptr(assert_test_store_max_bigdecimal_deltas_ptr, assert_test_store_max_bigdecimal_deltas_len).unwrap();

        Substreams::assert_all_test_bigdecimal(assert_test_store_add_bigdecimal,
            assert_test_store_add_bigdecimal_deltas,
            assert_test_store_set_bigdecimal,
            assert_test_store_set_bigdecimal_deltas,
            assert_test_store_set_if_not_exists_bigdecimal,
            assert_test_store_set_if_not_exists_bigdecimal_deltas,
            assert_test_store_min_bigdecimal,
            assert_test_store_min_bigdecimal_deltas,
            assert_test_store_max_bigdecimal,
            assert_test_store_max_bigdecimal_deltas,
            store,
        )
    };
    func()
}

#[no_mangle]
pub extern "C" fn assert_all_test_string(
    assert_test_store_append_string_ptr: *mut u8,
    assert_test_store_append_string_len: usize,
    assert_test_store_append_string_deltas_ptr: *mut u8,
    assert_test_store_append_string_deltas_len: usize,
    assert_test_store_set_string_ptr: *mut u8,
    assert_test_store_set_string_len: usize,
    assert_test_store_set_string_deltas_ptr: *mut u8,
    assert_test_store_set_string_deltas_len: usize,
    assert_test_store_set_if_not_exists_string_ptr: *mut u8,
    assert_test_store_set_if_not_exists_string_len: usize,
    assert_test_store_set_if_not_exists_string_deltas_ptr: *mut u8,
    assert_test_store_set_if_not_exists_string_deltas_len: usize,
) {
    substreams::register_panic_hook();
    let func = ||{
        
        let store: substreams::store::StoreSetInt64 = substreams::store::StoreSetInt64::new();
        
        let assert_test_store_append_string: pb::test::Boolean = substreams::proto::decode_ptr(assert_test_store_append_string_ptr, assert_test_store_append_string_len).unwrap();
        let assert_test_store_append_string_deltas: pb::test::Boolean = substreams::proto::decode_ptr(assert_test_store_append_string_deltas_ptr, assert_test_store_append_string_deltas_len).unwrap();
        let assert_test_store_set_string: pb::test::Boolean = substreams::proto::decode_ptr(assert_test_store_set_string_ptr, assert_test_store_set_string_len).unwrap();
        let assert_test_store_set_string_deltas: pb::test::Boolean = substreams::proto::decode_ptr(assert_test_store_set_string_deltas_ptr, assert_test_store_set_string_deltas_len).unwrap();
        let assert_test_store_set_if_not_exists_string: pb::test::Boolean = substreams::proto::decode_ptr(assert_test_store_set_if_not_exists_string_ptr, assert_test_store_set_if_not_exists_string_len).unwrap();
        let assert_test_store_set_if_not_exists_string_deltas: pb::test::Boolean = substreams::proto::decode_ptr(assert_test_store_set_if_not_exists_string_deltas_ptr, assert_test_store_set_if_not_exists_string_deltas_len).unwrap();

        Substreams::assert_all_test_string(assert_test_store_append_string,
            assert_test_store_append_string_deltas,
            assert_test_store_set_string,
            assert_test_store_set_string_deltas,
            assert_test_store_set_if_not_exists_string,
            assert_test_store_set_if_not_exists_string_deltas,
            store,
        )
    };
    func()
}

#[no_mangle]
pub extern "C" fn assert_all_test_delete_prefix(
    assert_test_store_delete_prefix_ptr: *mut u8,
    assert_test_store_delete_prefix_len: usize,
) {
    substreams::register_panic_hook();
    let func = ||{
        
        let store: substreams::store::StoreSetInt64 = substreams::store::StoreSetInt64::new();
        
        let assert_test_store_delete_prefix: pb::test::Boolean = substreams::proto::decode_ptr(assert_test_store_delete_prefix_ptr, assert_test_store_delete_prefix_len).unwrap();

        Substreams::assert_all_test_delete_prefix(assert_test_store_delete_prefix,
            store,
        )
    };
    func()
}

#[no_mangle]
pub extern "C" fn assert_all_test(
    assert_all_test_delete_prefix_ptr: u32,
    assert_all_test_string_ptr: u32,
    assert_all_test_i64_ptr: u32,
    assert_all_test_float64_ptr: u32,
    assert_all_test_bigint_ptr: u32,
    assert_all_test_bigdecimal_ptr: u32,
) {
    substreams::register_panic_hook();
    let func = ||-> Result<pb::test::Boolean, Error>{
        
        let assert_all_test_delete_prefix: substreams::store::StoreGetInt64 = substreams::store::StoreGetInt64::new(assert_all_test_delete_prefix_ptr);
        let assert_all_test_string: substreams::store::StoreGetInt64 = substreams::store::StoreGetInt64::new(assert_all_test_string_ptr);
        let assert_all_test_i64: substreams::store::StoreGetInt64 = substreams::store::StoreGetInt64::new(assert_all_test_i64_ptr);
        let assert_all_test_float64: substreams::store::StoreGetInt64 = substreams::store::StoreGetInt64::new(assert_all_test_float64_ptr);
        let assert_all_test_bigint: substreams::store::StoreGetInt64 = substreams::store::StoreGetInt64::new(assert_all_test_bigint_ptr);
        let assert_all_test_bigdecimal: substreams::store::StoreGetInt64 = substreams::store::StoreGetInt64::new(assert_all_test_bigdecimal_ptr);

        Substreams::assert_all_test(assert_all_test_delete_prefix,
            assert_all_test_string,
            assert_all_test_i64,
            assert_all_test_float64,
            assert_all_test_bigint,
            assert_all_test_bigdecimal,
            
        )
    };
    let result = func();
    if result.is_err() {
        panic!("{:?}", &result.err().unwrap());
    }
    substreams::output(result.unwrap());
}
