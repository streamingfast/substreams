use num_bigint::BigUint;

#[no_mangle]
extern "C" fn test_sum_big_int() {
    substreams::state::sum_big_int(1, "test.key.1".to_string(), BigUint::parse_bytes(b"10", 10).unwrap());
}
