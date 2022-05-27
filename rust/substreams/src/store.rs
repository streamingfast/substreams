use crate::pb;
use crate::state;
use bigdecimal::BigDecimal;
use num_bigint::BigInt;
use substreams_macro::StoreWriter;

pub type Deltas = Vec<pb::substreams::StoreDelta>;

/// `StoreSet` is a struct representing a `store` module defined with an
/// `updatePolicy` equal to `set` in the manifest.
#[derive(StoreWriter)]
pub struct StoreSet {}
impl StoreSet {
    pub fn set(&self, ord: u64, key: String, value: &Vec<u8>) {
        state::set(ord as i64, key, value);
    }
    pub fn set_many(&self, ord: u64, keys: &Vec<String>, value: &Vec<u8>) {
        for key in keys {
            state::set(ord as i64, key.to_string(), value);
        }
    }
}

/// `StoreSetIfNotExists` is a struct representing a `store` module defined with an
/// `updatePolicy` equal to `set_if_not_exists` in the manifest.
#[derive(StoreWriter)]
pub struct StoreSetIfNotExists {}
impl StoreSetIfNotExists {
    pub fn set_if_not_exists(&self, ord: u64, key: String, value: &Vec<u8>) {
        state::set_if_not_exists(ord as i64, key, value);
    }
    pub fn set_if_not_exists_many(&self, ord: u64, keys: &Vec<String>, value: &Vec<u8>) {
        for key in keys {
            state::set_if_not_exists(ord as i64, key.to_string(), value);
        }
    }
}

/// `StoreAddInt64` is a struct representing a `store` module defined with an
/// `updatePolicy` equal to `add` and a valueType of `int64`  in the manifest.
#[derive(StoreWriter)]
pub struct StoreAddInt64 {}
impl StoreAddInt64 {
    pub fn add(&self, ord: u64, key: String, value: i64) {
        state::add_int64(ord as i64, key, value);
    }
    pub fn add_many(&self, ord: u64, keys: &Vec<String>, value: i64) {
        for key in keys {
            state::add_int64(ord as i64, key.to_string(), value);
        }
    }
}

/// `StoreAddFloat64` is a struct representing a `store` module defined with an
/// `updatePolicy` equal to `add` and a valueType of `float64`  in the manifest.
#[derive(StoreWriter)]
pub struct StoreAddFloat64 {}
impl StoreAddFloat64 {
    pub fn add(&self, ord: u64, key: String, value: f64) {
        state::add_float64(ord as i64, key, value);
    }
    pub fn add_many(&self, ord: u64, keys: &Vec<String>, value: f64) {
        for key in keys {
            state::add_float64(ord as i64, key.to_string(), value);
        }
    }
}

/// `StoreAddBigFloat` is a struct representing a `store` module defined with an
/// `updatePolicy` equal to `add` and a valueType of `bigfloat`  in the manifest.
#[derive(StoreWriter)]
pub struct StoreAddBigFloat {}
impl StoreAddBigFloat {
    pub fn add(&self, ord: u64, key: String, value: &BigDecimal) {
        state::add_bigfloat(ord as i64, key, value);
    }
    pub fn add_many(&self, ord: u64, keys: &Vec<String>, value: &BigDecimal) {
        for key in keys {
            state::add_bigfloat(ord as i64, key.to_string(), value);
        }
    }
}

/// `StoreAddBigInt` is a struct representing a `store` module defined with an
/// `updatePolicy` equal to `add` and a valueType of `bigint`  in the manifest.
#[derive(StoreWriter)]
pub struct StoreAddBigInt {}
impl StoreAddBigInt {
    pub fn add(&self, ord: u64, key: String, value: &BigInt) {
        state::add_bigint(ord as i64, key, value);
    }
}

/// `StoreMaxInt64` is a struct representing a `store` module defined with an
/// `updatePolicy` equal to `max` and a valueType of `int64`  in the manifest.
#[derive(StoreWriter)]
pub struct StoreMaxInt64 {}
impl StoreMaxInt64 {
    pub fn max(&self, ord: u64, key: String, value: i64) {
        state::set_max_int64(ord as i64, key, value);
    }
}

/// `StoreMaxBigInt` is a struct representing a `store` module defined with an
/// `updatePolicy` equal to `max` and a valueType of `bigint`  in the manifest.
#[derive(StoreWriter)]
pub struct StoreMaxBigInt {}
impl StoreMaxBigInt {
    pub fn max(&self, ord: u64, key: String, value: &BigInt) {
        state::set_max_bigint(ord as i64, key, value);
    }
}

/// `StoreMaxFloat64` is a struct representing a `store` module defined with an
/// `updatePolicy` equal to `max` and a valueType of `float64`  in the manifest.
#[derive(StoreWriter)]
pub struct StoreMaxFloat64 {}
impl StoreMaxFloat64 {
    pub fn max(&self, ord: u64, key: String, value: f64) {
        state::set_max_float64(ord as i64, key, value);
    }
}

/// `StoreMaxBigFloat` is a struct representing a `store` module defined with an
/// `updatePolicy` equal to `max` and a valueType of `bigfloat`  in the manifest.
#[derive(StoreWriter)]
pub struct StoreMaxBigFloat {}
impl StoreMaxBigFloat {
    pub fn max(&self, ord: u64, key: String, value: &BigDecimal) {
        state::set_max_bigfloat(ord as i64, key, value);
    }
}

/// `StoreMinInt64` is a struct representing a `store` module defined with an
/// `updatePolicy` equal to `min` and a valueType of `int64`  in the manifest.
#[derive(StoreWriter)]
pub struct StoreMinInt64 {}
impl StoreMinInt64 {
    pub fn min(&self, ord: u64, key: String, value: i64) {
        state::set_min_int64(ord as i64, key, value);
    }
}

/// `StoreMinBigInt` is a struct representing a `store` module defined with an
/// `updatePolicy` equal to `min` and a valueType of `bigint`  in the manifest.
#[derive(StoreWriter)]
pub struct StoreMinBigInt {}
impl StoreMinBigInt {
    pub fn min(&self, ord: u64, key: String, value: &BigInt) {
        state::set_min_bigint(ord as i64, key, value);
    }
}

/// `StoreMinFloat64` is a struct representing a `store` module defined with an
/// `updatePolicy` equal to `min` and a valueType of `float64`  in the manifest.
#[derive(StoreWriter)]
pub struct StoreMinFloat64 {}
impl StoreMinFloat64 {
    pub fn min(&self, ord: u64, key: String, value: f64) {
        state::set_min_float64(ord as i64, key, value);
    }
}

/// `StoreMinBigFloat` is a struct representing a `store` module defined with an
/// `updatePolicy` equal to `min` and a valueType of `bigfloat`  in the manifest.
#[derive(StoreWriter)]
pub struct StoreMinBigFloat {}
impl StoreMinBigFloat {
    pub fn min(&self, ord: u64, key: String, value: &BigDecimal) {
        state::set_min_bigfloat(ord as i64, key, value);
    }
}

pub struct StoreGet {
    idx: u32,
}

impl StoreGet {
    pub fn new(idx: u32) -> StoreGet {
        StoreGet { idx }
    }
    pub fn get_at(&self, ord: u64, key: &String) -> Option<Vec<u8>> {
        return state::get_at(self.idx, ord as i64, key);
    }
    pub fn get_last(&self, key: &String) -> Option<Vec<u8>> {
        return state::get_last(self.idx, key);
    }
    pub fn get_first(&self, key: &String) -> Option<Vec<u8>> {
        return state::get_first(self.idx, key);
    }
}
