use crate::externs;
use crate::memory::memory;
use num_bigint::BigUint;
use bigdecimal::BigDecimal;

pub fn get_at(store_idx: u32, ord: i64, key: String) -> Option<Vec<u8>> {
    unsafe {
        let key_bytes = key.as_bytes();
        let output_ptr = memory::alloc(8);
        let found = externs::state::get_at(
            store_idx,
            ord,
            key_bytes.as_ptr(),
            key_bytes.len() as u32,
            output_ptr as u32,
        );
        return if found == 1 {
            Some(memory::get_output_data(output_ptr))
        } else {
            None
        };
    }
}

pub fn get_last(store_idx: u32, key: String) -> Option<Vec<u8>> {
    unsafe {
        let key_bytes = key.as_bytes();
        let output_ptr = memory::alloc(8);
        let found = externs::state::get_last(
            store_idx,
            key_bytes.as_ptr(),
            key_bytes.len() as u32,
            output_ptr as u32,
        );

        return if found == 1 {
            Some(memory::get_output_data(output_ptr))
        } else {
            None
        };
    }
}

pub fn get_first(store_idx: u32, key: String) -> Option<Vec<u8>> {
    unsafe {
        let key_bytes = key.as_bytes();
        let output_ptr = memory::alloc(8);
        let found = externs::state::get_first(
            store_idx,
            key_bytes.as_ptr(),
            key_bytes.len() as u32,
            output_ptr as u32,
        );

        return if found == 1 {
            Some(memory::get_output_data(output_ptr))
        } else {
            None
        };
    }
}

pub fn set(ord: i64, key: String, value: Vec<u8>) {
    unsafe {
        externs::state::set(
            ord,
            key.as_ptr(),
            key.len() as u32,
            value.as_ptr(),
            value.len() as u32,
        )
    }
}

pub fn sum_bigint(ord: i64, key: String, value: BigUint) {
    let data = value.to_string();
    unsafe {
        externs::state::sum_bigint(
            ord,
            key.as_ptr(),
            key.len() as u32,
            data.as_ptr(),
            data.len() as u32,
        )
    }
}


pub fn sum_int64(ord: i64, key: String, value: i64) {
    unsafe {
        externs::state::sum_int64(
            ord,
            key.as_ptr(),
            key.len() as u32,
	        value,
        )
    }
}

pub fn sum_float64(ord: i64, key: String, value: f64) {
    unsafe {
        externs::state::sum_float64(
            ord,
            key.as_ptr(),
            key.len() as u32,
            value
        )
    }
}

pub fn sum_bigfloat(ord: i64, key: String, value: BigDecimal) {
    let data = value.to_string();
    unsafe {
        externs::state::sum_bigfloat(
            ord,
            key.as_ptr(),
            key.len() as u32,
            data.as_ptr(),
            data.len() as u32,
        )
    }
}
