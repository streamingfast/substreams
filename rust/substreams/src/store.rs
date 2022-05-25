use crate::state;
use num_bigint::{BigInt};
use bigdecimal::BigDecimal;
use substreams_macro::StoreWriter;
/// `UpdateWriter` is a struct representing a `store` module defined with an
/// `updatePolicy` equal to `replace` in the manifest.
#[derive(StoreWriter)]
pub struct UpdateWriter { }
impl UpdateWriter {
    pub fn set(&self, ord: i64, key: String, value: &Vec<u8>) {
        state::set(ord,key,value);
    }
}

/// `ConditionalWriter` is a struct representing a `store` module defined with an
/// `updatePolicy` equal to `ignore` in the manifest.
#[derive(StoreWriter)]
pub struct ConditionalWriter { }
impl ConditionalWriter {
    pub fn set_if_not_exists(&self, ord: i64, key: String, value: &Vec<u8>) {
        state::set_if_not_exists(ord, key, value);
    }
}

/// `SumInt64Writer` is a struct representing a `store` module defined with an
/// `updatePolicy` equal to `sum` and a valueType of `int64`  in the manifest.
#[derive(StoreWriter)]
pub struct SumInt64Writer { }
impl SumInt64Writer {
    pub fn sum(&self, ord: i64, key: String, value: i64) {
        state::sum_int64(ord,key,value);
    }
}

/// `SumFloat64Writer` is a struct representing a `store` module defined with an
/// `updatePolicy` equal to `sum` and a valueType of `float64`  in the manifest.
#[derive(StoreWriter)]
pub struct SumFloat64Writer { }
impl SumFloat64Writer {
    pub fn sum(&self, ord: i64, key: String, value: f64) {
        state::sum_float64(ord,key,value);
    }
}

/// `SumBigFloatWriter` is a struct representing a `store` module defined with an
/// `updatePolicy` equal to `sum` and a valueType of `bigfloat`  in the manifest.
#[derive(StoreWriter)]
pub struct SumBigFloatWriter { }
impl SumBigFloatWriter {
    pub fn sum(&self, ord: i64, key: String, value: &BigDecimal) {
        state::sum_bigfloat(ord,key,value);
    }
}

/// `SumBigIntWriter` is a struct representing a `store` module defined with an
/// `updatePolicy` equal to `sum` and a valueType of `bigint`  in the manifest.
#[derive(StoreWriter)]
pub struct SumBigIntWriter { }
impl SumBigIntWriter {
    pub fn sum(&self, ord: i64, key: String, value: &BigInt) {
        state::sum_bigint(ord,key,value);
    }
}

/// `MaxInt64Writer` is a struct representing a `store` module defined with an
/// `updatePolicy` equal to `max` and a valueType of `int64`  in the manifest.
#[derive(StoreWriter)]
pub struct MaxInt64Writer {}
impl MaxInt64Writer {
    pub fn max(&self, ord: i64, key: String, value: i64) {
        state::set_max_int64(ord, key, value);
    }
}

/// `MaxBigIntWriter` is a struct representing a `store` module defined with an
/// `updatePolicy` equal to `max` and a valueType of `bigint`  in the manifest.
#[derive(StoreWriter)]
pub struct MaxBigIntWriter {}
impl MaxBigIntWriter {
    pub fn max(&self, ord: i64, key: String, value: &BigInt){
        state::set_max_bigint(ord, key, value);
    }
}

/// `MaxFloat64Writer` is a struct representing a `store` module defined with an
/// `updatePolicy` equal to `max` and a valueType of `float64`  in the manifest.
#[derive(StoreWriter)]
pub struct MaxFloat64Writer {}
impl MaxFloat64Writer {
    pub fn max(&self, ord: i64, key: String, value: f64){
        state::set_max_float64(ord, key, value);
    }
}

/// `MaxBigFloatWriter` is a struct representing a `store` module defined with an
/// `updatePolicy` equal to `max` and a valueType of `bigfloat`  in the manifest.
#[derive(StoreWriter)]
pub struct MaxBigFloatWriter {}
impl MaxBigFloatWriter {
    pub fn max(&self, ord: i64, key: String, value: &BigDecimal){
        state::set_max_bigfloat(ord, key, value);
    }
}

/// `MinInt64Writer` is a struct representing a `store` module defined with an
/// `updatePolicy` equal to `min` and a valueType of `int64`  in the manifest.
#[derive(StoreWriter)]
pub struct MinInt64Writer{}
impl MinInt64Writer {
    pub fn min(&self, ord: i64, key: String, value: i64) {
        state::set_min_int64(ord, key, value);
    }
}

/// `MinBigIntWriter` is a struct representing a `store` module defined with an
/// `updatePolicy` equal to `min` and a valueType of `bigint`  in the manifest.
#[derive(StoreWriter)]
pub struct MinBigIntWriter{}
impl MinBigIntWriter {
    pub fn min(&self, ord: i64, key: String, value: &BigInt) {
        state::set_min_bigint(ord, key, value);
    }
}

/// `MinFloat64Writer` is a struct representing a `store` module defined with an
/// `updatePolicy` equal to `min` and a valueType of `float64`  in the manifest.
#[derive(StoreWriter)]
pub struct MinFloat64Writer{}
impl MinFloat64Writer {
    pub fn min(&self, ord: i64, key: String, value: f64) {
        state::set_min_float64(ord, key, value);
    }
}

/// `MinBigFloatWriter` is a struct representing a `store` module defined with an
/// `updatePolicy` equal to `min` and a valueType of `bigfloat`  in the manifest.
#[derive(StoreWriter)]
pub struct MinBigFloatWriter{}
impl MinBigFloatWriter {
    pub fn min(&self, ord: i64, key: String, value: &BigDecimal) {
        state::set_min_bigfloat(ord, key, value);
    }
}

pub struct Reader{
    idx: u32
}

impl Reader {
    pub fn new(idx: u32) -> Reader { Reader{ idx } }

    pub fn get_at(&self, ord: i64, key: &String) -> Option<Vec<u8>> {
        return state::get_at(self.idx, ord, key);

    }

    pub fn get_last(&self, key: &String) -> Option<Vec<u8>> {
        return state::get_last(self.idx, key);
    }

    pub fn get_first(&self, key: &String) -> Option<Vec<u8>> {
        return state::get_first(self.idx, key);
    }
}


