use crate::externs;
use crate::memory;
use bigdecimal::BigDecimal;
use num_bigint::BigInt;

pub fn get_at<K: AsRef<str>>(store_idx: u32, ord: i64, key: K) -> Option<Vec<u8>> {
    let key = key.as_ref();

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
pub fn get_last<K: AsRef<str>>(store_idx: u32, key: K) -> Option<Vec<u8>> {
    let key = key.as_ref();

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
pub fn get_first<K: AsRef<str>>(store_idx: u32, key: K) -> Option<Vec<u8>> {
    let key = key.as_ref();

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
pub fn set<K: AsRef<str>>(ord: i64, key: K, value: &Vec<u8>) {
    let key = key.as_ref();

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
pub fn set_if_not_exists<K: AsRef<str>>(ord: i64, key: K, value: &Vec<u8>) {
    let key = key.as_ref();

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

pub fn append<K: AsRef<str>>(ord: i64, key: K, value: &Vec<u8>) {
    let key = key.as_ref();

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

pub fn delete_prefix<K: AsRef<str>>(ord: i64, prefix: K) {
    let prefix = prefix.as_ref();

    unsafe { externs::state::delete_prefix(ord, prefix.as_ptr(), prefix.len() as u32) }
}

pub fn add_bigint<K: AsRef<str>>(ord: i64, key: K, value: &BigInt)  {
    let key = key.as_ref();
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
pub fn add_int64<K: AsRef<str>>(ord: i64, key: K, value: i64) {
    let key = key.as_ref();

    unsafe { externs::state::add_int64(ord, key.as_ptr(), key.len() as u32, value) }
}

pub fn add_float64<K: AsRef<str>>(ord: i64, key: K, value: f64) {
    let key = key.as_ref();

    unsafe { externs::state::add_float64(ord, key.as_ptr(), key.len() as u32, value) }
}

pub fn add_bigfloat<K: AsRef<str>>(ord: i64, key: K, value: &BigDecimal) {
    let key = key.as_ref();
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

pub fn set_min_int64<K: AsRef<str>>(ord: i64, key: K, value: i64) {
    let key = key.as_ref();

    unsafe { externs::state::set_min_int64(ord, key.as_ptr(), key.len() as u32, value) }
}

pub fn set_min_bigint<K: AsRef<str>>(ord: i64, key: K, value: &BigInt) {
    let key = key.as_ref();
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

pub fn set_min_float64<K: AsRef<str>>(ord: i64, key: K, value: f64) {
    let key = key.as_ref();

    unsafe { externs::state::set_min_float64(ord, key.as_ptr(), key.len() as u32, value) }
}

pub fn set_min_bigfloat<K: AsRef<str>>(ord: i64, key: K, value: &BigDecimal) {
    let key = key.as_ref();
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

pub fn set_max_int64<K: AsRef<str>>(ord: i64, key: K, value: i64) {
    let key = key.as_ref();

    unsafe { externs::state::set_max_int64(ord, key.as_ptr(), key.len() as u32, value) }
}

pub fn set_max_bigint<K: AsRef<str>>(ord: i64, key: K, value: &BigInt) {
    let key = key.as_ref();
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

pub fn set_max_float64<K: AsRef<str>>(ord: i64, key: K, value: f64) {
    let key = key.as_ref();

    unsafe { externs::state::set_max_float64(ord, key.as_ptr(), key.len() as u32, value) }
}

pub fn set_max_bigfloat<K: AsRef<str>>(ord: i64, key: K, value: &BigDecimal) {
    let key = key.as_ref();
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
