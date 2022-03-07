#[no_mangle]
extern "C" fn test_sum_big_int() {
    substreams::state::sum_int64(1, "test.key.1".to_string(), 10);
    substreams::state::sum_int64(1, "test.key.1".to_string(), 10);
}

#[no_mangle]
extern "C" fn test_sum_int64() {
    substreams::state::sum_int64(1, "sum.int.64".to_string(), 10);
}

#[no_mangle]
extern "C" fn test_sum_float64() {
    substreams::state::sum_float64(1, "sum.float.64".to_string(), 10.75)
}
