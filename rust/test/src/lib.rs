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
