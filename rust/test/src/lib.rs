use num_bigint::BigUint;

#[no_mangle]
extern "C" fn test_sum_big_int() {
    substreams::state::sum_int64(1, "test.key.1".to_string(), 10);
    substreams::state::sum_int64(1, "test.key.1".to_string(), 10);
}
