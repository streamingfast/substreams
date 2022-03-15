mod pb;

use crate::pb::substreams::RpcCall;
use bigdecimal::BigDecimal;
use num_bigint::BigUint;

#[no_mangle]
extern "C" fn test_sum_big_int() {
    substreams::state::sum_bigint(
        1,
        "test.key.1".to_string(),
        BigUint::parse_bytes(b"10", 10).unwrap(),
    );
    substreams::state::sum_bigint(
        1,
        "test.key.1".to_string(),
        BigUint::parse_bytes(b"10", 10).unwrap(),
    );
}

#[no_mangle]
extern "C" fn test_sum_int64() {
    substreams::state::sum_int64(1, "sum.int.64".to_string(), 10);
    substreams::state::sum_int64(1, "sum.int.64".to_string(), 10);
}

#[no_mangle]
extern "C" fn test_sum_float64() {
    substreams::state::sum_float64(1, "sum.float.64".to_string(), 10.75);
    substreams::state::sum_float64(1, "sum.float.64".to_string(), 10.75);
}

#[no_mangle]
extern "C" fn test_sum_big_float_small_number() {
    substreams::state::sum_bigfloat(
        1,
        "sum.big.float".to_string(),
        BigDecimal::parse_bytes(b"10.5", 10).unwrap(),
    );
    substreams::state::sum_bigfloat(
        1,
        "sum.big.float".to_string(),
        BigDecimal::parse_bytes(b"10.5", 10).unwrap(),
    );
}

#[no_mangle]
extern "C" fn test_sum_big_float_big_number() {
    substreams::state::sum_bigfloat(
        1,
        "sum.big.float".to_string(),
        BigDecimal::parse_bytes(b"12345678987654321.5", 10).unwrap(),
    );
    substreams::state::sum_bigfloat(
        1,
        "sum.big.float".to_string(),
        BigDecimal::parse_bytes(b"12345678987654321.5", 10).unwrap(),
    );
}

#[no_mangle]
extern "C" fn test_set_min_int64() {
    substreams::state::set_min_int64(1, "set_min_int64".to_string(), 5);
    substreams::state::set_min_int64(1, "set_min_int64".to_string(), 2);
}

#[no_mangle]
extern "C" fn test_set_min_bigint() {
    substreams::state::set_min_bigint(
        1,
        "set_min_bigint".to_string(),
        BigUint::parse_bytes(b"5", 10).unwrap(),
    );
    substreams::state::set_min_bigint(
        1,
        "set_min_bigint".to_string(),
        BigUint::parse_bytes(b"3", 10).unwrap(),
    );
}

#[no_mangle]
extern "C" fn test_set_min_float64() {
    substreams::state::set_min_float64(1, "set_min_float64".to_string(), 10.05);
    substreams::state::set_min_float64(1, "set_min_float64".to_string(), 10.04);
}

#[no_mangle]
extern "C" fn test_set_min_bigfloat() {
    substreams::state::set_min_bigfloat(
        1,
        "set_min_bigfloat".to_string(),
        BigDecimal::parse_bytes(b"11.05", 10).unwrap(),
    );
    substreams::state::set_min_bigfloat(
        1,
        "set_min_bigfloat".to_string(),
        BigDecimal::parse_bytes(b"11.04", 10).unwrap(),
    );
}

#[no_mangle]
extern "C" fn test_set_max_int64() {
    substreams::state::set_max_int64(1, "set_max_int64".to_string(), 5);
    substreams::state::set_max_int64(1, "set_max_int64".to_string(), 2);
}

#[no_mangle]
extern "C" fn test_set_max_bigint() {
    substreams::state::set_max_bigint(
        1,
        "set_max_bigint".to_string(),
        BigUint::parse_bytes(b"5", 10).unwrap(),
    );
    substreams::state::set_max_bigint(
        1,
        "set_max_bigint".to_string(),
        BigUint::parse_bytes(b"3", 10).unwrap(),
    );
}

#[no_mangle]
extern "C" fn test_set_max_float64() {
    substreams::state::set_max_float64(1, "set_max_float64".to_string(), 10.05);
    substreams::state::set_max_float64(1, "set_max_float64".to_string(), 10.04);
}

#[no_mangle]
extern "C" fn test_set_max_bigfloat() {
    substreams::state::set_max_bigfloat(
        1,
        "set_max_bigfloat".to_string(),
        BigDecimal::parse_bytes(b"11.05", 10).unwrap(),
    );
    substreams::state::set_max_bigfloat(
        1,
        "set_max_bigfloat".to_string(),
        BigDecimal::parse_bytes(b"11.04", 10).unwrap(),
    );
}

#[no_mangle]
extern "C" fn test_eth_call() {
    let deadbeef = hex::decode("deadbeef").unwrap();
    let addr = hex::decode("ea674fdde714fd979de3edf0f56aa9716b898ec8").unwrap();

    let rpc_calls = pb::substreams::RpcCalls {
        calls: vec![RpcCall {
            to_addr: addr,
            method_signature: deadbeef,
        }],
    };

    substreams::rpc::eth_call(substreams::proto::encode(&rpc_calls).unwrap());
}

#[no_mangle]
extern "C" fn test_eth_call_2() {
    let method_signature_1 = hex::decode("deadbeef").unwrap();
    let addr = hex::decode("ea674fdde714fd979de3edf0f56aa9716b898ec8").unwrap();

    let method_signature2 = hex::decode("beefdead").unwrap();
    let addr2 = hex::decode("0e09fabb73bd3ade0a17ecc321fd13a19e81ce82").unwrap();

    let calls = vec![
        RpcCall {
            to_addr: addr,
            method_signature: method_signature_1,
        },
        RpcCall {
            to_addr: addr2,
            method_signature: method_signature2,
        }
    ];

    let rpc_calls = pb::substreams::RpcCalls {
        calls,
    };

    substreams::rpc::eth_call(substreams::proto::encode(&rpc_calls).unwrap());
}

#[no_mangle]
extern "C" fn test_eth_call_3() {
    let method_signature1 = hex::decode("deadbeef").unwrap();
    let addr = hex::decode("ea674fdde714fd979de3edf0f56aa9716b898ec8").unwrap();

    let method_signature2 = hex::decode("beefdead").unwrap();
    let addr2 = hex::decode("0e09fabb73bd3ade0a17ecc321fd13a19e81ce82").unwrap();

    let method_signature3 = hex::decode("feebdead").unwrap();
    let addr3 = hex::decode("d006a7431be66fec522503db41f54692b85447c1").unwrap();

    let calls = vec![
        RpcCall {
            to_addr: addr,
            method_signature: method_signature1,
        },
        RpcCall {
            to_addr: addr2,
            method_signature: method_signature2,
        },
        RpcCall {
            to_addr: addr3,
            method_signature: method_signature3,
        }
    ];

    let rpc_calls = pb::substreams::RpcCalls {
        calls,
    };

    substreams::rpc::eth_call(substreams::proto::encode(&rpc_calls).unwrap());
}

#[no_mangle]
extern "C" fn test_set_delete_prefix() {
    substreams::state::set(1, "1:key_to_keep".to_string(), [1, 2, 3, 4].to_vec());
    substreams::state::set(2, "2:key_to_delete".to_string(), [5, 6, 7, 8].to_vec());
    substreams::state::delete_prefix(3, "2:".to_string());
}


#[no_mangle]
extern "C" fn test_make_it_crash(data_ptr: *mut u8, data_len: usize) {
     unsafe {
        let input_data = Vec::from_raw_parts(data_ptr, data_len, data_len);
        let cloned_data = input_data.clone();
        substreams::output_raw(cloned_data);
    };
}

#[no_mangle]
extern "C" fn test_memory_leak() {
    substreams::memory::alloc(10485760); // allocate 1MB on each call
}
