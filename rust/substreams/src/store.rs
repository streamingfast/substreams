//! Store Implementation for Substreams.
//!
//! This crate implements the different Stores which can be used in your Substreams
//! handlers.
//!

use crate::pb;
use crate::state;
use bigdecimal::BigDecimal;
use num_bigint::BigInt;
use substreams_macro::StoreWriter;

/// Delta is a struct that defined StoreDeltas
pub type Deltas = Vec<pb::substreams::StoreDelta>;

/// StoreSet is a struct representing a `store` with
/// `updatePolicy` equal to `set`
#[derive(StoreWriter)]
pub struct StoreSet {}
impl StoreSet {
    /// Set a given key to a given value, if the key existed before, it will be replaced.
    pub fn set<K: AsRef<str>>(&self, ord: u64, key: K, value: &Vec<u8>) {
        state::set(ord as i64, key, value);
    }

    /// Set many keys to a given values, if the key existed before, it will be replaced.
    pub fn set_many<K: AsRef<str>>(&self, ord: u64, keys: &Vec<K>, value: &Vec<u8>) {
        for key in keys {
            state::set(ord as i64, key, value);
        }
    }
}

/// StoreSetIfNotExists is a struct representing a `store` module with
/// `updatePolicy` equal to `set_if_not_exists`
#[derive(StoreWriter)]
pub struct StoreSetIfNotExists {}
impl StoreSetIfNotExists {
    /// Set a given key to a given value, if the key existed before, it will be ignored and not set.
    pub fn set_if_not_exists<K: AsRef<str>>(&self, ord: u64, key: K, value: &Vec<u8>) {
        state::set_if_not_exists(ord as i64, key, value);
    }

    /// Set given keys to given values, if the key existed before, it will be ignored and not set.
    pub fn set_if_not_exists_many<K: AsRef<str>>(&self, ord: u64, keys: &Vec<K>, value: &Vec<u8>) {
        for key in keys {
            state::set_if_not_exists(ord as i64, key, value);
        }
    }
}

/// StoreAddInt64 is a struct representing a `store` module with
/// `updatePolicy` equal to `add` and a valueType of `int64`
#[derive(StoreWriter)]
pub struct StoreAddInt64 {}
impl StoreAddInt64 {
    /// Will add the value to the already present value at the key (or default to
    /// zero if the key was not set)
    pub fn add<K: AsRef<str>>(&self, ord: u64, key: K, value: i64) {
        state::add_int64(ord as i64, key, value);
    }

    /// Will add the value to the already present value of the keys (or default to
    /// zero if the key was not set)
    pub fn add_many<K: AsRef<str>>(&self, ord: u64, keys: &Vec<K>, value: i64) {
        for key in keys {
            state::add_int64(ord as i64, key, value);
        }
    }
}

/// StoreAddFloat64 is a struct representing a `store` module with
/// `updatePolicy` equal to `add` and a valueType of `float64`
#[derive(StoreWriter)]
pub struct StoreAddFloat64 {}
impl StoreAddFloat64 {
    /// Will add the value to the already present value at the key (or default to
    /// zero if the key was not set)
    pub fn add<K: AsRef<str>>(&self, ord: u64, key: K, value: f64) {
        state::add_float64(ord as i64, key, value);
    }

    /// Will add the value to the already present value of the keys (or default to
    /// zero if the key was not set)
    pub fn add_many<K: AsRef<str>>(&self, ord: u64, keys: &Vec<K>, value: f64) {
        for key in keys {
            state::add_float64(ord as i64, key, value);
        }
    }
}

/// StoreAddBigFloat is a struct representing a `store` module with
/// `updatePolicy` equal to `add` and a valueType of `bigfloat`
#[derive(StoreWriter)]
pub struct StoreAddBigFloat {}
impl StoreAddBigFloat {
    /// Will add the value to the already present value at the key (or default to
    /// zero if the key was not set)
    pub fn add<K: AsRef<str>>(&self, ord: u64, key: K, value: &BigDecimal) {
        state::add_bigfloat(ord as i64, key, value);
    }

    /// Will add the value to the already present value of the keys (or default to
    /// zero if the key was not set)
    pub fn add_many<K: AsRef<str>>(&self, ord: u64, keys: &Vec<K>, value: &BigDecimal) {
        for key in keys {
            state::add_bigfloat(ord as i64, key, value);
        }
    }
}

/// StoreAddBigInt is a struct representing a `store` module with
/// `updatePolicy` equal to `add` and a valueType of `bigint`
#[derive(StoreWriter)]
pub struct StoreAddBigInt {}
impl StoreAddBigInt {
    /// Will add the value to the already present value of the keys (or default to
    /// zero if the key was not set)
    pub fn add<K: AsRef<str>>(&self, ord: u64, key: K, value: &BigInt) {
        state::add_bigint(ord as i64, key, value);
    }

    /// Will add the value to the already present value of the keys (or default to
    /// zero if the key was not set)
    pub fn add_many<K: AsRef<str>>(&self, ord: u64, keys: &Vec<K>, value: &BigInt) {
        for key in keys {
            state::add_bigint(ord as i64, key, value);
        }
    }
}

/// StoreMaxInt64 is a struct representing a `store` module with
/// `updatePolicy` equal to `max` and a valueType of `int64`
#[derive(StoreWriter)]
pub struct StoreMaxInt64 {}
impl StoreMaxInt64 {
    /// max will set the provided key in the store only if the value received in
    /// parameter is bigger than the one already present in the store, with
    /// a default of the zero value when the key is absent.
    pub fn max<K: AsRef<str>>(&self, ord: u64, key: K, value: i64) {
        state::set_max_int64(ord as i64, key, value);
    }
}

/// StoreMaxBigInt is a struct representing a `store` module with
/// `updatePolicy` equal to `max` and a valueType of `bigint`
#[derive(StoreWriter)]
pub struct StoreMaxBigInt {}
impl StoreMaxBigInt {
    /// Will set the provided key in the store only if the value received in
    /// parameter is bigger than the one already present in the store, with
    /// a default of the zero value when the key is absent.
    pub fn max<K: AsRef<str>>(&self, ord: u64, key: K, value: &BigInt) {
        state::set_max_bigint(ord as i64, key, value);
    }
}

/// StoreMaxFloat64 is a struct representing a `store` module with
/// `updatePolicy` equal to `max` and a valueType of `float64`
#[derive(StoreWriter)]
pub struct StoreMaxFloat64 {}
impl StoreMaxFloat64 {
    /// Will set the provided key in the store only if the value received in
    /// parameter is bigger than the one already present in the store, with
    /// a default of the zero value when the key is absent.
    pub fn max<K: AsRef<str>>(&self, ord: u64, key: K, value: f64) {
        state::set_max_float64(ord as i64, key, value);
    }
}

/// StoreMaxBigFloat is a struct representing a `store` module with
/// `updatePolicy` equal to `max` and a valueType of `bigfloat`
#[derive(StoreWriter)]
pub struct StoreMaxBigFloat {}
impl StoreMaxBigFloat {
    /// Will set the provided key in the store only if the value received in
    /// parameter is bigger than the one already present in the store, with
    /// a default of the zero value when the key is absent.
    pub fn max<K: AsRef<str>>(&self, ord: u64, key: K, value: &BigDecimal) {
        state::set_max_bigfloat(ord as i64, key, value);
    }
}

/// `StoreMinInt64` is a struct representing a `store` module with
/// `updatePolicy` equal to `min` and a valueType of `int64`
#[derive(StoreWriter)]
pub struct StoreMinInt64 {}
impl StoreMinInt64 {
    /// Will set the provided key in the store only if the value received in
    /// parameter is smaller than the one already present in the store, with
    /// a default of the zero value when the key is absent.
    pub fn min<K: AsRef<str>>(&self, ord: u64, key: K, value: i64) {
        state::set_min_int64(ord as i64, key, value);
    }
}

/// StoreMinBigInt is a struct representing a `store` module with
/// `updatePolicy` equal to `min` and a valueType of `bigint`
#[derive(StoreWriter)]
pub struct StoreMinBigInt {}
impl StoreMinBigInt {
    /// Will set the provided key in the store only if the value received in
    /// parameter is smaller than the one already present in the store, with
    /// a default of the zero value when the key is absent.
    pub fn min<K: AsRef<str>>(&self, ord: u64, key: K, value: &BigInt) {
        state::set_min_bigint(ord as i64, key, value);
    }
}

/// StoreMinFloat64 is a struct representing a `store` module with
/// `updatePolicy` equal to `min` and a valueType of `float64`
#[derive(StoreWriter)]
pub struct StoreMinFloat64 {}
impl StoreMinFloat64 {
    /// Will set the provided key in the store only if the value received in
    /// parameter is smaller than the one already present in the store, with
    /// a default of the zero value when the key is absent.
    pub fn min<K: AsRef<str>>(&self, ord: u64, key: K, value: f64) {
        state::set_min_float64(ord as i64, key, value);
    }
}

/// StoreMinBigFloat is a struct representing a `store` module with
/// `updatePolicy` equal to `min` and a valueType of `bigfloat`
#[derive(StoreWriter)]
pub struct StoreMinBigFloat {}
impl StoreMinBigFloat {
    /// Will set the provided key in the store only if the value received in
    /// parameter is smaller than the one already present in the store, with
    /// a default of the zero value when the key is absent.
    pub fn min<K: AsRef<str>>(&self, ord: u64, key: K, value: &BigDecimal) {
        state::set_min_bigfloat(ord as i64, key, value);
    }
}

/// StoreAppend is a struct representing a `store` with
/// `updatePolicy` equal to `append`
#[derive(StoreWriter)]
pub struct StoreAppend {}
impl StoreAppend {
    /// Concatenates a given value at the end of the key's current value
    pub fn append<K: AsRef<str>>(&self, ord: u64, key: K, value: &String) {
        state::append(ord as i64, key, &value.as_bytes().to_vec());
    }

    /// Concatenates a given value at the end of the key's current value
    pub fn append_bytes<K: AsRef<str>>(&self, ord: u64, key: K, value: &Vec<u8>) {
        state::append(ord as i64, key, value);
    }
}

/// StoreGet is a struct representing a read only store `store`
pub struct StoreGet {
    idx: u32,
}

impl StoreGet {
    /// Return a StoreGet object with a store index set
    pub fn new(idx: u32) -> StoreGet {
        StoreGet { idx }
    }

    /// Allows you to read a single key from the store. The type
    /// of its value can be anything, and is usually declared in
    /// the output section of the manifest. The ordinal is used here
    /// to go query a key that might have changed mid-block by
    /// the store module that built it.
    pub fn get_at<K: AsRef<str>>(&self, ord: u64, key: K) -> Option<Vec<u8>> {
        return state::get_at(self.idx, ord as i64, key);
    }

    /// Retrieves a key from the store, like `get_at`, but querying the state of
    /// the store as of the beginning of the block being processed, before any changes
    /// were applied within the current block. Tt does not need to rewind any changes
    /// in the middle of the block.
    pub fn get_last<K: AsRef<str>>(&self, key: K) -> Option<Vec<u8>> {
        return state::get_last(self.idx, key);
    }

    /// Retrieves a key from the store, like `get_at`, but querying the state of
    /// the store as of the beginning of the block being processed, before any changes
    /// were applied within the current block. However, it needs to unwind any keys that
    /// would have changed mid-block, so will be slightly less performant.
    pub fn get_first<K: AsRef<str>>(&self, key: K) -> Option<Vec<u8>> {
        return state::get_first(self.idx, key);
    }
}
