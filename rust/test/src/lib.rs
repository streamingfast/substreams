use bigdecimal::BigDecimal;
use num_bigint::BigUint;

#[no_mangle]
extern "C" fn test_sum_big_int() {
    substreams::state::sum_bigint(1, "test.key.1".to_string(), BigUint::parse_bytes(b"10", 10).unwrap());
    substreams::state::sum_bigint(1, "test.key.1".to_string(), BigUint::parse_bytes(b"10", 10).unwrap());
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
    substreams::state::sum_bigfloat(1, "sum.big.float".to_string(), BigDecimal::parse_bytes(b"10.5", 10).unwrap());
    substreams::state::sum_bigfloat(1, "sum.big.float".to_string(), BigDecimal::parse_bytes(b"10.5", 10).unwrap());
}

#[no_mangle]
extern "C" fn test_sum_big_float_big_number() {
    substreams::state::sum_bigfloat(1, "sum.big.float".to_string(), BigDecimal::parse_bytes(b"12345678987654321.5", 10).unwrap());
    substreams::state::sum_bigfloat(1, "sum.big.float".to_string(), BigDecimal::parse_bytes(b"12345678987654321.5", 10).unwrap());
}

#[no_mangle]
extern "C" fn test_set_min_int64() {
    substreams::state::set_min_int64(1, "set_min_int64".to_string(), 5);
    substreams::state::set_min_int64(1, "set_min_int64".to_string(), 2);
}

#[no_mangle]
extern "C" fn test_set_min_bigint() {
    substreams::state::set_min_bigint(1, "set_min_bigint".to_string(), BigUint::parse_bytes(b"5", 10).unwrap());
    substreams::state::set_min_bigint(1, "set_min_bigint".to_string(), BigUint::parse_bytes(b"3", 10).unwrap());
}

#[no_mangle]
extern "C" fn test_set_min_float64() {
    substreams::state::set_min_float64(1, "set_min_float64".to_string(), 10.05);
    substreams::state::set_min_float64(1, "set_min_float64".to_string(), 10.04);
}

#[no_mangle]
extern "C" fn test_set_min_bigfloat() {
    substreams::state::set_min_bigfloat(1, "set_min_bigfloat".to_string(), BigDecimal::parse_bytes(b"11.05", 10).unwrap());
    substreams::state::set_min_bigfloat(1, "set_min_bigfloat".to_string(), BigDecimal::parse_bytes(b"11.04", 10).unwrap());
}

#[no_mangle]
extern "C" fn test_set_max_int64() {
    substreams::state::set_max_int64(1, "set_max_int64".to_string(), 5);
    substreams::state::set_max_int64(1, "set_max_int64".to_string(), 2);
}

#[no_mangle]
extern "C" fn test_set_max_bigint() {
    substreams::state::set_max_bigint(1, "set_max_bigint".to_string(), BigUint::parse_bytes(b"5", 10).unwrap());
    substreams::state::set_max_bigint(1, "set_max_bigint".to_string(), BigUint::parse_bytes(b"3", 10).unwrap());
}

#[no_mangle]
extern "C" fn test_set_max_float64() {
    substreams::state::set_max_float64(1, "set_max_float64".to_string(), 10.05);
    substreams::state::set_max_float64(1, "set_max_float64".to_string(), 10.04);
}

#[no_mangle]
extern "C" fn test_set_max_bigfloat() {
    substreams::state::set_max_bigfloat(1, "set_max_bigfloat".to_string(), BigDecimal::parse_bytes(b"11.05", 10).unwrap());
    substreams::state::set_max_bigfloat(1, "set_max_bigfloat".to_string(), BigDecimal::parse_bytes(b"11.04", 10).unwrap());
}

#[no_mangle]
extern "C" fn test_set_delete_prefix() {
    substreams::state::set(1, "1:key_to_keep".to_string(), [1, 2, 3, 4].to_vec());
    substreams::state::set(2, "2:key_to_delete".to_string(), [5, 6, 7, 8].to_vec());
    substreams::state::delete_prefix(3, "2:".to_string());
}
