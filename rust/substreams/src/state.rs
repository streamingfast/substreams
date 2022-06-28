use crate::externs;
use crate::memory;
use num_bigint::{BigInt};
use bigdecimal::BigDecimal;

pub fn get_at(store_idx: u32, ord: i64, key: &String) -> Option<Vec<u8>> {
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
pub fn get_last(store_idx: u32, key: &String) -> Option<Vec<u8>> {
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
pub fn get_first(store_idx: u32, key: &String) -> Option<Vec<u8>> {
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
pub fn set(ord: i64, key: String, value: &Vec<u8>) {
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
pub fn set_if_not_exists(ord: i64, key: String, value: &Vec<u8>) {
    unsafe {
        externs::state::set_if_not_exists(
            ord,
            key.as_ptr(),
            key.len() as u32,
            value.as_ptr(),
            value.len() as u32,
        )
    }
}

pub fn append(ord: i64, key: String, value: &Vec<u8>) {
    unsafe {
        externs::state::append(
            ord,
            key.as_ptr(),
            key.len() as u32,
            value.as_ptr(),
            value.len() as u32,
        )
    }
}

pub fn delete_prefix(ord: i64, prefix: &String){
    unsafe {
        externs::state::delete_prefix(
            ord,
            prefix.as_ptr(),
            prefix.len() as u32,
        )
    }
}
pub fn add_bigint(ord: i64, key: String, value: &BigInt) {
    let data = value.to_string();
    unsafe {
        externs::state::add_bigint(
            ord,
            key.as_ptr(),
            key.len() as u32,
            data.as_ptr(),
            data.len() as u32,
        )
    }
}
pub fn add_int64(ord: i64, key: String, value: i64) {
    unsafe {
        externs::state::add_int64(
            ord,
            key.as_ptr(),
            key.len() as u32,
	        value,
        )
    }
}
pub fn add_float64(ord: i64, key: String, value: f64) {
    unsafe {
        externs::state::add_float64(
            ord,
            key.as_ptr(),
            key.len() as u32,
            value
        )
    }
}
pub fn add_bigfloat(ord: i64, key: String, value: &BigDecimal) {
    let data = value.to_string();
    unsafe {
        externs::state::add_bigfloat(
            ord,
            key.as_ptr(),
            key.len() as u32,
            data.as_ptr(),
            data.len() as u32,
        )
    }
}
pub fn set_min_int64(ord: i64, key: String, value: i64) {
    unsafe {
        externs::state::set_min_int64(
            ord,
            key.as_ptr(),
            key.len() as u32,
            value,
        )
    }
}
pub fn set_min_bigint(ord: i64, key: String, value: &BigInt) {
    let data = value.to_string();
    unsafe {
        externs::state::set_min_bigint(
            ord,
            key.as_ptr(),
            key.len() as u32,
            data.as_ptr(),
            data.len() as u32,
        )
    }
}
pub fn set_min_float64(ord: i64, key: String, value: f64) {
    unsafe {
        externs::state::set_min_float64(
            ord,
            key.as_ptr(),
            key.len() as u32,
            value,
        )
    }
}
pub fn set_min_bigfloat(ord: i64, key: String, value: &BigDecimal) {
    let data = value.to_string();
    unsafe {
        externs::state::set_min_bigfloat(
            ord,
            key.as_ptr(),
            key.len() as u32,
            data.as_ptr(),
            data.len() as u32,
        )
    }
}
pub fn set_max_int64(ord: i64, key: String, value: i64) {
    unsafe {
        externs::state::set_max_int64(
            ord,
            key.as_ptr(),
            key.len() as u32,
            value,
        )
    }
}
pub fn set_max_bigint(ord: i64, key: String, value: &BigInt) {
    let data = value.to_string();
    unsafe {
        externs::state::set_max_bigint(
            ord,
            key.as_ptr(),
            key.len() as u32,
            data.as_ptr(),
            data.len() as u32,
        )
    }
}
pub fn set_max_float64(ord: i64, key: String, value: f64) {
    unsafe {
        externs::state::set_max_float64(
            ord,
            key.as_ptr(),
            key.len() as u32,
            value,
        )
    }
}
pub fn set_max_bigfloat(ord: i64, key: String, value: &BigDecimal) {
    let data = value.to_string();
    unsafe {
        externs::state::set_max_bigfloat(
            ord,
            key.as_ptr(),
            key.len() as u32,
            data.as_ptr(),
            data.len() as u32,
        )
    }
}
