mod generated;

use std::borrow::Borrow;
use substreams::prelude::*;
use substreams::errors::Error;
use std::fmt::{Debug, Display};
use std::ops::Deref;
use prost::encoding::float;

use substreams::{
    errors,
    scalar::BigInt,
    store::{
        DeltaBigInt, DeltaInt64, Deltas, StoreAdd, StoreAddBigInt, StoreAddInt64, StoreDelete,
        StoreGet, StoreGetBigInt, StoreGetInt64, StoreMinInt64, StoreNew, StoreSet, StoreSetInt64,
        StoreSetProto, StoreSetIfNotExists, StoreSetIfNotExistsInt64, StoreSetIfNotExistsProto,
    },
};
use substreams::scalar::BigDecimal;
use substreams::store::{DeltaBigDecimal, DeltaFloat64, StoreAddBigDecimal, StoreAddFloat64, StoreGetBigDecimal, StoreGetFloat64, StoreMaxBigDecimal, StoreMaxBigInt, StoreMaxFloat64, StoreMaxInt64, StoreMinBigDecimal, StoreMinBigInt, StoreMinFloat64, StoreSetBigDecimal, StoreSetBigInt, StoreSetFloat64};

use crate::pb::test;
use crate::pb::test::Block;

mod pb;

const TO_SET: i64 = 100;
const TO_ADD: i64 = 1;
const TO_SUBTRACT: i64 = -1;

impl generated::substreams::SubstreamsTrait for generated::substreams::Substreams {
    fn test_map(blk: test::Block) -> Result<test::MapResult, errors::Error> {
        let out = test::MapResult {
            block_number: blk.number,
            block_hash: blk.id,
        };
        Ok(out)
    }

    fn test_store_proto(map_result: test::MapResult, s: StoreSetProto<test::MapResult>) {
        let key = format!("result:{}", map_result.block_hash);
        s.set(1, key, &map_result)
    }

    fn test_store_delete_prefix(block: test::Block, s: StoreSetInt64) {
        let to_set_key = format!("key:{}", block.number);
        s.set(block.number, to_set_key, &TO_SET);

        if block.number > 1 {
            let previous_block_num = block.number - 1;
            let to_delete_key = format!("key:{}", previous_block_num);
            s.delete_prefix(block.number as i64, &to_delete_key)
        }
    }

    fn assert_test_store_delete_prefix(block: test::Block, s: StoreGetInt64) -> Result<bool, errors::Error> {
        let to_read_key = format!("key:{}", block.number);
        assert_eq!(TO_SET, s.get_last(to_read_key).unwrap());

        if block.number > 1 {
            let previous_block_num = block.number - 1;
            let deleted_key = format!("key:{}", previous_block_num);
            assert_eq!(None, s.get_last(deleted_key))
        }

        Ok(true)
    }

    ////////////////////// INT 64 //////////////////////

    fn setup_test_store_add_i64(block: test::Block, s: StoreAddInt64) {
        s.add(block.number, "a.key", i64::MAX);
        s.add(block.number, "a.key", i64::MIN);
        s.add(block.number, "a.key", 1);
    }

    fn assert_test_store_add_i64(block: test::Block, s: StoreGetInt64) -> Result<bool, errors::Error> {
        assert(block.number, 0, s.get_last("a.key").unwrap());
        Ok(true)
    }

    fn assert_test_store_add_i64_deltas(block: test::Block, _store: StoreGetInt64, deltas: Deltas<DeltaInt64>, ) -> Result<bool, errors::Error> {
        if deltas.deltas.len() != 3 {
            panic!("expected 3 deltas, got {}", deltas.deltas.len());
        }

        let delta_0 = deltas.deltas.get(0).unwrap();
        assert(block.number, 0, delta_0.old_value);
        assert(block.number, i64::MAX, delta_0.new_value);

        let delta_1 = deltas.deltas.get(1).unwrap();
        assert(block.number, i64::MAX, delta_1.old_value);
        assert(block.number, -1, delta_1.new_value);

        let delta_2 = deltas.deltas.get(2).unwrap();
        assert(block.number, -1, delta_2.old_value);
        assert(block.number, 0, delta_2.new_value);

        Ok(true)
    }

    fn setup_test_store_min_i64(block: test::Block, s: StoreMinInt64) {
        match block.number {
            1 => {
                s.min(block.number, "a.key", i64::MIN);
                s.min(block.number, "a.key", i64::MAX);
            },
            _ => {
                s.min(block.number, "a.key", i64::MIN);
            }
        }
    }

    fn assert_test_store_min_i64(block: test::Block, s: StoreGetInt64) -> Result<bool, errors::Error> {
        assert(block.number, i64::MIN, s.get_last("a.key").unwrap());
        Ok(true)
    }

    fn assert_test_store_min_i64_deltas(block: test::Block, _store: StoreGetInt64, deltas: Deltas<DeltaInt64>, ) -> Result<bool, errors::Error> {
        match block.number {
            1 => {
                if deltas.deltas.len() != 2 {
                    panic!("expected 2 deltas, got {}", deltas.deltas.len());
                }

                let delta_0 = deltas.deltas.get(0).unwrap();
                assert(1, 0, delta_0.old_value);
                assert(1, i64::MIN, delta_0.new_value);

                let delta_1 = deltas.deltas.get(1).unwrap();
                assert(block.number, i64::MIN, delta_1.old_value);
                assert(block.number, i64::MIN, delta_1.new_value);
            },
            _ => {
                if deltas.deltas.len() != 1 {
                    panic!("expected 1 delta, got {}", deltas.deltas.len());
                }

                let delta_0 = deltas.deltas.get(0).unwrap();
                assert(1, i64::MIN, delta_0.old_value);
                assert(1, i64::MIN, delta_0.new_value);
            }
        }

        Ok(true)
    }

    fn setup_test_store_max_i64(block: Block, s: StoreMaxInt64) {
        match block.number {
            1 => {
                s.max(block.number, "a.key", i64::MAX);
                s.max(block.number, "a.key", i64::MIN);
            },
            _ => {
                s.max(block.number, "a.key", i64::MAX);
            }
        }
    }

    fn assert_test_store_max_i64(block: Block, s: substreams::store::StoreGetInt64) -> Result<bool, Error> {
        assert(block.number, i64::MAX, s.get_last("a.key").unwrap());
        Ok(true)
    }

    fn assert_test_store_max_i64_deltas(block: Block, _store: substreams::store::StoreGetInt64, deltas: Deltas<substreams::store::DeltaInt64>) -> Result<bool, Error> {
        match block.number {
            1 => {
                if deltas.deltas.len() != 2 {
                    panic!("expected 2 deltas, got {}", deltas.deltas.len());
                }

                let delta_0 = deltas.deltas.get(0).unwrap();
                assert(block.number, 0, delta_0.old_value);
                assert(block.number, i64::MAX, delta_0.new_value);

                let delta_1 = deltas.deltas.get(1).unwrap();
                assert(block.number, i64::MAX, delta_1.old_value);
                assert(block.number, i64::MAX, delta_1.new_value);
            },
            _ => {
                if deltas.deltas.len() != 1 {
                    panic!("expected 1 delta, got {}", deltas.deltas.len());
                }

                let delta_0 = deltas.deltas.get(0).unwrap();
                assert(block.number, i64::MAX, delta_0.old_value);
                assert(block.number, i64::MAX, delta_0.new_value);
            }
        }

        Ok(true)
    }

    fn setup_test_store_set_i64(block: test::Block, store: StoreSetInt64) {
        store.set(block.number, "0", &0);
        store.set(block.number, "min", &i64::MIN);
        store.set(block.number, "max", &i64::MAX);
    }

    fn assert_test_store_set_i64(block: test::Block, s: StoreGetInt64) -> Result<bool, errors::Error> {
        assert(block.number, 0, s.get_last("0").unwrap());
        assert(block.number, i64::MIN, s.get_last("min").unwrap());
        assert(block.number, i64::MAX, s.get_last("max").unwrap());
        Ok(true)
    }

    fn assert_test_store_set_i64_deltas(block: test::Block, s: StoreGetInt64, deltas: Deltas<DeltaInt64>) -> Result<bool, errors::Error> {
        if deltas.deltas.len() != 3 {
            panic!("expected 3 deltas, got {}", deltas.deltas.len());
        }

        let delta_0 = deltas.deltas.get(0).unwrap();
        assert(block.number, 0, delta_0.new_value);
        let delta_1 = deltas.deltas.get(1).unwrap();
        assert(block.number, i64::MIN, delta_1.new_value);
        let delta_2 = deltas.deltas.get(2).unwrap();
        assert(block.number, i64::MAX, delta_2.new_value);

        Ok(true)
    }

    fn setup_test_store_set_if_not_exists_i64(block: Block, s: StoreSetIfNotExistsInt64) {
        s.set_if_not_exists(block.number, "key.0", &10);
        s.set_if_not_exists(block.number, "key.0", &1000);
    }

    fn assert_test_store_set_if_not_exists_i64(block: Block, s: StoreGetInt64) -> Result<bool, Error> {
        assert(block.number, 10, s.get_last("key.0").unwrap());
        Ok(true)
    }

    fn assert_test_store_set_if_not_exists_i64_deltas(block: Block, s: StoreGetInt64, deltas: Deltas<DeltaInt64>) -> Result<bool, Error> {
        match block.number {
            1 => {
                if deltas.deltas.len() != 1 {
                    panic!("expected 1 delta, got {}", deltas.deltas.len());
                }

                let delta_0 = deltas.deltas.get(0).unwrap();
                assert(block.number, 0, delta_0.old_value);
                assert(block.number, 10, delta_0.new_value);
            },
            _ => {
                if deltas.deltas.len() != 0 {
                    panic!("expected 0 deltas, got {}", deltas.deltas.len());
                }
            }
        }

        Ok(true)
    }

    ////////////////////// FLOAT 64 //////////////////////

    fn setup_test_store_add_float64(block: Block, s: StoreAddFloat64) {
        s.add(block.number, "a.key", 1.0);
        s.add(block.number, "a.key", 0.0);
        s.add(block.number, "a.key", -1.0);
    }

    fn assert_test_store_add_float64(block: Block, s: StoreGetFloat64) -> Result<bool, Error> {
        let value = s.get_last("a.key").unwrap();
        assert(block.number, 0.0, value);
        Ok(true)
    }

    fn assert_test_store_add_float64_deltas(block: Block, s: StoreGetFloat64, deltas: Deltas<DeltaFloat64>) -> Result<bool, Error> {
        if deltas.deltas.len() != 3 {
            panic!("expected 3 deltas, got {}", deltas.deltas.len());
        }

        let delta_0 = deltas.deltas.get(0).unwrap();
        assert(block.number, 0.0, delta_0.old_value);
        assert(block.number, 1.0, delta_0.new_value);

        let delta_1 = deltas.deltas.get(1).unwrap();
        assert(block.number, 1.0, delta_1.old_value);
        assert(block.number, 1.0, delta_1.new_value);

        let delta_2 = deltas.deltas.get(2).unwrap();
        assert(block.number, 1.0, delta_2.old_value);
        assert(block.number, 0.0, delta_2.new_value);

        Ok(true)
    }

    fn setup_test_store_set_float64(block: Block, store: StoreSetFloat64) {
        store.set(block.number, "0", &0.0);
        store.set(block.number, "min", &f64::MIN);
        store.set(block.number, "max", &f64::MAX);
    }

    fn setup_test_store_set_if_not_exists_float64(block: Block, s: StoreSetIfNotExistsFloat64) {
        s.set_if_not_exists(block.number, "key.0", &10.0);
        s.set_if_not_exists(block.number, "key.0", &1000.0);
    }

    fn assert_test_store_set_if_not_exists_float64(block: Block, s: StoreGetFloat64) -> Result<bool, Error> {
        assert(block.number, 10.0, s.get_last("key.0").unwrap());
        Ok(true)
    }

    fn assert_test_store_set_if_not_exists_float64_deltas(block: Block, s: StoreGetFloat64, deltas: Deltas<DeltaFloat64>) -> Result<bool, Error> {
        match block.number {
            1 => {
                if deltas.deltas.len() != 1 {
                    panic!("expected 1 delta, got {}", deltas.deltas.len());
                }

                let delta_0 = deltas.deltas.get(0).unwrap();
                assert(block.number, 0.0, delta_0.old_value);
                assert(block.number, 10.0, delta_0.new_value);
            },
            _ => {
                if deltas.deltas.len() != 0 {
                    panic!("expected 0 deltas, got {}", deltas.deltas.len());
                }
            }
        }

        Ok(true)
    }

    fn assert_test_store_set_float64(block: Block, s: StoreGetFloat64) -> Result<bool, Error> {
        assert(block.number, 0.0, s.get_last("0").unwrap());
        assert(block.number, f64::MIN, s.get_last("min").unwrap());
        assert(block.number, f64::MAX, s.get_last("max").unwrap());
        Ok(true)
    }

    fn assert_test_store_set_float64_deltas(block: Block, setup_test_store_set_float64: StoreGetFloat64, deltas: Deltas<DeltaFloat64>) -> Result<bool, Error> {
        if deltas.deltas.len() != 3 {
            panic!("expected 3 deltas, got {}", deltas.deltas.len());
        }

        let delta_0 = deltas.deltas.get(0).unwrap();
        assert(block.number, 0.0, delta_0.new_value);
        let delta_1 = deltas.deltas.get(1).unwrap();
        assert(block.number, f64::MIN, delta_1.new_value);
        let delta_2 = deltas.deltas.get(2).unwrap();
        assert(block.number, f64::MAX, delta_2.new_value);

        Ok(true)
    }

    fn setup_test_store_min_float64(block: Block, s: StoreMinFloat64) {
        match block.number {
            1 => {
                s.min(block.number, "a.key", f64::MIN);
                s.min(block.number, "a.key", f64::MAX);
            },
            _ => {
                s.min(block.number, "a.key", f64::MIN);
            }
        }
    }

    fn assert_test_store_min_float64(block: Block, s: StoreGetFloat64) -> Result<bool, Error> {
        assert(block.number, f64::MIN, s.get_last("a.key").unwrap());
        Ok(true)
    }

    fn assert_test_store_min_float64_deltas(block: Block, s: StoreGetFloat64, deltas: Deltas<DeltaFloat64>) -> Result<bool, Error> {
        match block.number {
            1 => {
                if deltas.deltas.len() != 2 {
                    panic!("expected 2 deltas, got {}", deltas.deltas.len());
                }

                let delta_0 = deltas.deltas.get(0).unwrap();
                assert(block.number, 0.0, delta_0.old_value);
                assert(block.number, f64::MIN, delta_0.new_value);

                let delta_1 = deltas.deltas.get(1).unwrap();
                assert(block.number, f64::MIN, delta_1.old_value);
                assert(block.number, f64::MIN, delta_1.new_value);
            },
            _ => {
                if deltas.deltas.len() != 1 {
                    panic!("expected 1 delta, got {}", deltas.deltas.len());
                }

                let delta_0 = deltas.deltas.get(0).unwrap();
                assert(block.number, f64::MIN, delta_0.old_value);
                assert(block.number, f64::MIN, delta_0.new_value);
            }
        }

        Ok(true)
    }

    fn setup_test_store_max_float64(block: Block, s: StoreMaxFloat64) {
        match block.number {
            1 => {
                s.max(block.number, "a.key", f64::MAX);
                s.max(block.number, "a.key", f64::MIN);
            },
            _ => {
                s.max(block.number, "a.key", f64::MAX);
            }
        }
    }

    fn assert_test_store_max_float64(block: Block, s: StoreGetFloat64) -> Result<bool, Error> {
        assert(block.number, f64::MAX, s.get_last("a.key").unwrap());
        Ok(true)
    }

    fn assert_test_store_max_float64_deltas(block: Block, s: StoreGetFloat64, deltas: Deltas<DeltaFloat64>) -> Result<bool, Error> {
        match block.number {
            1 => {
                if deltas.deltas.len() != 2 {
                    panic!("expected 2 deltas, got {}", deltas.deltas.len());
                }

                let delta_0 = deltas.deltas.get(0).unwrap();
                assert(block.number, 0.0, delta_0.old_value);
                assert(block.number, f64::MAX, delta_0.new_value);

                let delta_1 = deltas.deltas.get(1).unwrap();
                assert(block.number, f64::MAX, delta_1.old_value);
                assert(block.number, f64::MAX, delta_1.new_value);
            },
            _ => {
                if deltas.deltas.len() != 1 {
                    panic!("expected 1 delta, got {}", deltas.deltas.len());
                }

                let delta_0 = deltas.deltas.get(0).unwrap();
                assert(block.number, f64::MAX, delta_0.old_value);
                assert(block.number, f64::MAX, delta_0.new_value);
            }
        }

        Ok(true)
    }

    ////////////////////// BIG INT //////////////////////

    fn setup_test_store_add_bigint(block: Block, s: StoreAddBigInt) {
        s.add(block.number, "a.key", BigInt::from(1));
        s.add(block.number, "a.key", BigInt::from(0));
        s.add(block.number, "a.key", BigInt::from(-1));
    }

    fn assert_test_store_add_bigint(block: Block, s: StoreGetBigInt) -> Result<bool, Error> {
        let value = s.get_last("a.key").unwrap();
        assert(block.number, BigInt::from(0), value);
        Ok(true)
    }

    fn assert_test_store_add_bigint_deltas(block: Block, s: StoreGetBigInt, deltas: Deltas<DeltaBigInt>) -> Result<bool, Error> {
        if deltas.deltas.len() != 3 {
            panic!("expected 3 deltas, got {}", deltas.deltas.len());
        }

        let delta_0 = deltas.deltas.get(0).unwrap();
        assert(block.number, &BigInt::from(0), delta_0.old_value.borrow());
        assert(block.number, &BigInt::from(1), delta_0.new_value.borrow());

        let delta_1 = deltas.deltas.get(1).unwrap();
        assert(block.number, &BigInt::from(1), delta_1.old_value.borrow());
        assert(block.number, &BigInt::from(1), delta_1.new_value.borrow());

        let delta_2 = deltas.deltas.get(2).unwrap();
        assert(block.number, &BigInt::from(1), delta_2.old_value.borrow());
        assert(block.number, &BigInt::from(0), delta_2.new_value.borrow());

        Ok(true)
    }

    fn setup_test_store_set_bigint(block: Block, store: StoreSetBigInt) {
        store.set(block.number, "0", &BigInt::from(0));
        store.set(block.number, "min", &BigInt::from(i64::MIN));
        store.set(block.number, "max", &BigInt::from(i64::MAX));
    }

    fn assert_test_store_set_bigint(block: Block, s: StoreGetBigInt) -> Result<bool, Error> {
        assert(block.number, &BigInt::from(i64::from(0)), s.get_last("0").unwrap().borrow());
        assert(block.number, &BigInt::from(i64::MIN), s.get_last("min").unwrap().borrow());
        assert(block.number, &BigInt::from(i64::MAX), s.get_last("max").unwrap().borrow());
        Ok(true)
    }

    fn assert_test_store_set_bigint_deltas(block: Block, s: StoreGetBigInt, deltas: Deltas<DeltaBigInt>) -> Result<bool, Error> {
        if deltas.deltas.len() != 3 {
            panic!("expected 3 deltas, got {}", deltas.deltas.len());
        }

        let delta_0 = deltas.deltas.get(0).unwrap();
        assert(block.number, &BigInt::from(i64::from(0)), delta_0.new_value.borrow());
        let delta_1 = deltas.deltas.get(1).unwrap();
        assert(block.number, &BigInt::from(i64::MIN), delta_1.new_value.borrow());
        let delta_2 = deltas.deltas.get(2).unwrap();
        assert(block.number, &BigInt::from(i64::MAX), delta_2.new_value.borrow());

        Ok(true)
    }

    fn setup_test_store_set_if_not_exists_bigint(block: Block, store: StoreSetIfNotExistsBigInt) {
        store.set_if_not_exists(block.number, "key.a", &BigInt::from(10));
        store.set_if_not_exists(block.number, "key.a", &BigInt::from(1000));
    }

    fn assert_test_store_set_if_not_exists_bigint(block: Block, s: StoreGetBigInt) -> Result<bool, Error> {
        assert(block.number, &BigInt::from(i64::from(10)), s.get_last("key.a").unwrap().borrow());
        Ok(true)
    }

    fn assert_test_store_set_if_not_exists_bigint_deltas(block: Block, s: StoreGetBigInt, deltas: Deltas<DeltaBigInt>) -> Result<bool, Error> {
        match block.number {
            1 => {
                if deltas.deltas.len() != 1 {
                    panic!("expected 1 delta, got {}", deltas.deltas.len());
                }

                let delta_0 = deltas.deltas.get(0).unwrap();
                assert(block.number, &BigInt::from(i64::from(0)), delta_0.old_value.borrow());
                assert(block.number, &BigInt::from(i64::from(10)), delta_0.new_value.borrow());
            },
            _ => {
                if deltas.deltas.len() != 0 {
                    panic!("expected 0 deltas, got {}", deltas.deltas.len());
                }
            }
        }

        Ok(true)
    }

    fn setup_test_store_min_bigint(block: Block, s: StoreMinBigInt) {
        match block.number {
            1 => {
                s.min(block.number, "a.key", BigInt::from(-1));
                s.min(block.number, "a.key", BigInt::from(1));
            },
            _ => {
                s.min(block.number, "a.key", BigInt::from(-1));
            }
        }
    }

    fn assert_test_store_min_bigint(block: Block, s: StoreGetBigInt) -> Result<bool, Error> {
        assert(block.number, BigInt::from(-1), s.get_last("a.key").unwrap());
        Ok(true)
    }

    fn assert_test_store_min_bigint_deltas(block: Block, s: StoreGetBigInt, deltas: Deltas<DeltaBigInt>) -> Result<bool, Error> {
        match block.number {
            1 => {
                if deltas.deltas.len() != 2 {
                    panic!("expected 2 deltas, got {}", deltas.deltas.len());
                }

                let delta_0 = deltas.deltas.get(0).unwrap();
                assert(block.number, &BigInt::from(i64::from(0)), delta_0.old_value.borrow());
                assert(block.number, &BigInt::from(i64::from(-1)), delta_0.new_value.borrow());

                let delta_1 = deltas.deltas.get(1).unwrap();
                assert(block.number, &BigInt::from(i64::from(-1)), delta_1.old_value.borrow());
                assert(block.number, &BigInt::from(i64::from(-1)), delta_1.new_value.borrow());
            },
            _ => {
                if deltas.deltas.len() != 1 {
                    panic!("expected 1 delta, got {}", deltas.deltas.len());
                }

                let delta_0 = deltas.deltas.get(0).unwrap();
                assert(block.number, &BigInt::from(i64::from(-1)), delta_0.old_value.borrow());
                assert(block.number, &BigInt::from(i64::from(-1)), delta_0.new_value.borrow());
            }
        }

        Ok(true)
    }

    fn setup_test_store_max_bigint(block: Block, s: StoreMaxBigInt) {
        match block.number {
            1 => {
                s.max(block.number, "a.key", BigInt::from(1));
                s.max(block.number, "a.key", BigInt::from(-1));
            },
            _ => {
                s.max(block.number, "a.key", BigInt::from(1));
            }
        }
    }

    fn assert_test_store_max_bigint(block: Block, s: StoreGetBigInt) -> Result<bool, Error> {
        assert(block.number, BigInt::from(1), s.get_last("a.key").unwrap());
        Ok(true)
    }

    fn assert_test_store_max_bigint_deltas(block: Block, s: StoreGetBigInt, deltas: Deltas<DeltaBigInt>) -> Result<bool, Error> {
        match block.number {
            1 => {
                if deltas.deltas.len() != 2 {
                    panic!("expected 2 deltas, got {}", deltas.deltas.len());
                }

                let delta_0 = deltas.deltas.get(0).unwrap();
                assert(block.number, &BigInt::from(i64::from(0)), delta_0.old_value.borrow());
                assert(block.number, &BigInt::from(i64::from(1)), delta_0.new_value.borrow());

                let delta_1 = deltas.deltas.get(1).unwrap();
                assert(block.number, &BigInt::from(i64::from(1)), delta_1.old_value.borrow());
                assert(block.number, &BigInt::from(i64::from(1)), delta_1.new_value.borrow());
            },
            _ => {
                if deltas.deltas.len() != 1 {
                    panic!("expected 1 delta, got {}", deltas.deltas.len());
                }

                let delta_0 = deltas.deltas.get(0).unwrap();
                assert(block.number, &BigInt::from(i64::from(1)), delta_0.old_value.borrow());
                assert(block.number, &BigInt::from(i64::from(1)), delta_0.new_value.borrow());
            }
        }

        Ok(true)
    }

    ////////////////////// BIG DECIMAL //////////////////////

    fn setup_test_store_add_bigdecimal(block: Block, s: StoreAddBigDecimal) {
        s.add(block.number, "a.key", BigDecimal::from(1));
        s.add(block.number, "a.key", BigDecimal::from(0));
        s.add(block.number, "a.key", BigDecimal::from(-1));
    }

    fn assert_test_store_add_bigdecimal(block: Block, s: StoreGetBigDecimal) -> Result<bool, Error> {
        let value = s.get_last("a.key").unwrap();
        assert(block.number, BigDecimal::from(0), value);
        Ok(true)
    }

    fn assert_test_store_add_bigdecimal_deltas(block: Block, s: StoreGetBigDecimal, deltas: Deltas<DeltaBigDecimal>) -> Result<bool, Error> {
        if deltas.deltas.len() != 3 {
            panic!("expected 3 deltas, got {}", deltas.deltas.len());
        }

        let delta_0 = deltas.deltas.get(0).unwrap();
        assert(block.number, &BigDecimal::from(0), delta_0.old_value.borrow());
        assert(block.number, &BigDecimal::from(1), delta_0.new_value.borrow());

        let delta_1 = deltas.deltas.get(1).unwrap();
        assert(block.number, &BigDecimal::from(1), delta_1.old_value.borrow());
        assert(block.number, &BigDecimal::from(1), delta_1.new_value.borrow());

        let delta_2 = deltas.deltas.get(2).unwrap();
        assert(block.number, &BigDecimal::from(1), delta_2.old_value.borrow());
        assert(block.number, &BigDecimal::from(0), delta_2.new_value.borrow());

        Ok(true)
    }

    fn setup_test_store_set_bigdecimal(block: Block, s: StoreSetBigDecimal) {
        s.set(block.number, "0", &BigDecimal::from(0));
        s.set(block.number, "min", &BigDecimal::from(i64::MIN));
        s.set(block.number, "max", &BigDecimal::from(i64::MAX));
    }

    fn assert_test_store_set_bigdecimal(block: Block, s: StoreGetBigDecimal) -> Result<bool, Error> {
        assert(block.number, &BigDecimal::from(i64::from(0)), s.get_last("0").unwrap().borrow());
        assert(block.number, &BigDecimal::from(i64::MIN), s.get_last("min").unwrap().borrow());
        assert(block.number, &BigDecimal::from(i64::MAX), s.get_last("max").unwrap().borrow());
        Ok(true)
    }

    fn assert_test_store_set_bigdecimal_deltas(block: Block, s: StoreGetBigDecimal, deltas: Deltas<DeltaBigDecimal>) -> Result<bool, Error> {
        if deltas.deltas.len() != 3 {
            panic!("expected 3 deltas, got {}", deltas.deltas.len());
        }

        let delta_0 = deltas.deltas.get(0).unwrap();
        assert(block.number, &BigDecimal::from(i64::from(0)), delta_0.new_value.borrow());
        let delta_1 = deltas.deltas.get(1).unwrap();
        assert(block.number, &BigDecimal::from(i64::MIN), delta_1.new_value.borrow());
        let delta_2 = deltas.deltas.get(2).unwrap();
        assert(block.number, &BigDecimal::from(i64::MAX), delta_2.new_value.borrow());

        Ok(true)
    }

    fn setup_test_store_set_if_not_exists_bigdecimal(block: Block, store: StoreSetIfNotExistsBigDecimal) {
        store.set_if_not_exists(block.number, "key.a", &BigDecimal::from(10));
        store.set_if_not_exists(block.number, "key.a", &BigDecimal::from(1000));
    }

    fn assert_test_store_set_if_not_exists_bigdecimal(block: Block, s: StoreGetBigDecimal) -> Result<bool, Error> {
        assert(block.number, &BigDecimal::from(i64::from(10)), s.get_last("key.a").unwrap().borrow());
        Ok(true)
    }

    fn assert_test_store_set_if_not_exists_bigdecimal_deltas(block: Block, s: StoreGetBigDecimal, deltas: Deltas<DeltaBigDecimal>) -> Result<bool, Error> {
        match block.number {
            1 => {
                if deltas.deltas.len() != 1 {
                    panic!("expected 1 delta, got {}", deltas.deltas.len());
                }

                let delta_0 = deltas.deltas.get(0).unwrap();
                assert(block.number, &BigDecimal::from(i64::from(0)), delta_0.old_value.borrow());
                assert(block.number, &BigDecimal::from(i64::from(10)), delta_0.new_value.borrow());
            },
            _ => {
                if deltas.deltas.len() != 0 {
                    panic!("expected 0 deltas, got {}", deltas.deltas.len());
                }
            }
        }

        Ok(true)
    }

    fn setup_test_store_min_bigdecimal(block: Block, s: StoreMinBigDecimal) {
        match block.number {
            1 => {
                s.min(block.number, "a.key", BigDecimal::from(-1));
                s.min(block.number, "a.key", BigDecimal::from(1));
            },
            _ => {
                s.min(block.number, "a.key", BigDecimal::from(-1));
            }
        }
    }

    fn assert_test_store_min_bigdecimal(block: Block, s: StoreGetBigDecimal) -> Result<bool, Error> {
        assert(block.number, BigDecimal::from(-1), s.get_last("a.key").unwrap());
        Ok(true)
    }

    fn assert_test_store_min_bigdecimal_deltas(block: Block, s: StoreGetBigDecimal, deltas: Deltas<DeltaBigDecimal>) -> Result<bool, Error> {
        match block.number {
            1 => {
                if deltas.deltas.len() != 2 {
                    panic!("expected 2 deltas, got {}", deltas.deltas.len());
                }

                let delta_0 = deltas.deltas.get(0).unwrap();
                assert(block.number, &BigDecimal::from(i64::from(0)), delta_0.old_value.borrow());
                assert(block.number, &BigDecimal::from(i64::from(-1)), delta_0.new_value.borrow());

                let delta_1 = deltas.deltas.get(1).unwrap();
                assert(block.number, &BigDecimal::from(i64::from(-1)), delta_1.old_value.borrow());
                assert(block.number, &BigDecimal::from(i64::from(-1)), delta_1.new_value.borrow());
            },
            _ => {
                if deltas.deltas.len() != 1 {
                    panic!("expected 1 delta, got {}", deltas.deltas.len());
                }

                let delta_0 = deltas.deltas.get(0).unwrap();
                assert(block.number, &BigDecimal::from(i64::from(-1)), delta_0.old_value.borrow());
                assert(block.number, &BigDecimal::from(i64::from(-1)), delta_0.new_value.borrow());
            }
        }

        Ok(true)
    }

    fn setup_test_store_max_bigdecimal(block: Block, s: StoreMaxBigDecimal) {
        match block.number {
            1 => {
                s.max(block.number, "a.key", BigDecimal::from(1));
                s.max(block.number, "a.key", BigDecimal::from(-1));
            },
            _ => {
                s.max(block.number, "a.key", BigDecimal::from(1));
            }
        }
    }

    fn assert_test_store_max_bigdecimal(block: Block, s: StoreGetBigDecimal) -> Result<bool, Error> {
        assert(block.number, BigDecimal::from(1), s.get_last("a.key").unwrap());
        Ok(true)
    }

    fn assert_test_store_max_bigdecimal_deltas(block: Block, s: StoreGetBigDecimal, deltas: Deltas<DeltaBigDecimal>) -> Result<bool, Error> {
        match block.number {
            1 => {
                if deltas.deltas.len() != 2 {
                    panic!("expected 2 deltas, got {}", deltas.deltas.len());
                }

                let delta_0 = deltas.deltas.get(0).unwrap();
                assert(block.number, &BigDecimal::from(i64::from(0)), delta_0.old_value.borrow());
                assert(block.number, &BigDecimal::from(i64::from(1)), delta_0.new_value.borrow());

                let delta_1 = deltas.deltas.get(1).unwrap();
                assert(block.number, &BigDecimal::from(i64::from(1)), delta_1.old_value.borrow());
                assert(block.number, &BigDecimal::from(i64::from(1)), delta_1.new_value.borrow());
            },
            _ => {
                if deltas.deltas.len() != 1 {
                    panic!("expected 1 delta, got {}", deltas.deltas.len());
                }

                let delta_0 = deltas.deltas.get(0).unwrap();
                assert(block.number, &BigDecimal::from(i64::from(1)), delta_0.old_value.borrow());
                assert(block.number, &BigDecimal::from(i64::from(1)), delta_0.new_value.borrow());
            }
        }

        Ok(true)
    }

    ////////////////////// STRING //////////////////////

    fn setup_test_store_append_string(block: Block, store: StoreAppend<String>) {
        store.append(block.number, "test.key", "a".to_string());
    }

    fn assert_test_store_append_string(block: Block, store: StoreGetRaw) -> Result<bool, Error> {
        match block.number {
            1 => {
                let raw_value = store.get_last("test.key").unwrap();
                let value = String::from_utf8(raw_value).unwrap();
                assert(block.number, "a;", value.as_str());
            },
            3 => {
                let raw_value = store.get_last("test.key").unwrap();
                let value = String::from_utf8(raw_value).unwrap();
                assert(block.number, "a;a;a;", value.as_str());
            }
            _ => {}
        }

        Ok(true)
    }

    fn assert_test_store_append_string_deltas(block: Block, s: StoreGetRaw, deltas: Deltas<DeltaArray<String>>) -> Result<bool, Error> {
        match block.number {
            1 => {
                if deltas.deltas.len() != 1 {
                    panic!("expected 1 delta, got {}", deltas.deltas.len());
                }

                let delta_0 = deltas.deltas.get(0).unwrap();

                let old_value = delta_0.old_value.clone().to_vec().join(";");
                let empty_vec : Vec<String> = vec!();
                let old_expected_value = empty_vec.join(";");
                assert(block.number, old_expected_value, old_value);

                let new_value = delta_0.new_value.clone().to_vec().join(";");
                let new_expected_value = vec!("a").join(";");
                assert(block.number, new_expected_value, new_value);
            },
            3 => {
                if deltas.deltas.len() != 1 {
                    panic!("expected 1 delta, got {}", deltas.deltas.len());
                }

                let delta_0 = deltas.deltas.get(0).unwrap();

                let old_value = delta_0.old_value.clone().to_vec().join(";");
                let old_expected_value = vec!("a", "a").join(";");
                assert(block.number, old_expected_value, old_value);

                let new_value = delta_0.new_value.clone().to_vec().join(";");
                let new_expected_value = vec!("a", "a", "a").join(";");
                assert(block.number, new_expected_value, new_value);
            }
            _ => {}
        }

        Ok(true)
    }

    fn setup_test_store_set_string(block: Block, store: StoreSetString) {
        store.set(block.number, "a.key", &"foo".to_string());
    }

    fn assert_test_store_set_string(block: Block, store: StoreGetString) -> Result<bool, Error> {
        let value = store.get_last("a.key").unwrap();
        assert(block.number, "foo", value.as_str());
        Ok(true)
    }

    fn assert_test_store_set_string_deltas(block: Block, store: StoreGetString, deltas: Deltas<DeltaString>) -> Result<bool, Error> {
        match block.number {
            1 => {
                if deltas.deltas.len() != 1 {
                    panic!("expected 1 delta, got {}", deltas.deltas.len());
                }

                let delta_0 = deltas.deltas.get(0).unwrap();
                assert(block.number, "", delta_0.old_value.as_str());
                assert(block.number, "foo", delta_0.new_value.as_str());
            },
            _ => {
                if deltas.deltas.len() != 1 {
                    panic!("expected 1 delta, got {}", deltas.deltas.len());
                }

                let delta_0 = deltas.deltas.get(0).unwrap();
                assert(block.number, "foo", delta_0.old_value.as_str());
                assert(block.number, "foo", delta_0.new_value.as_str());
            }
        }

        Ok(true)
    }

    fn setup_test_store_set_if_not_exists_string(block: Block, store: StoreSetIfNotExistsString) {
        store.set_if_not_exists(block.number, "a.key", &"foo".to_string());
        store.set_if_not_exists(block.number, "a.key", &"bar".to_string());
    }

    fn assert_test_store_set_if_not_exists_string(block: Block, store: StoreGetString) -> Result<bool, Error> {
        let value = store.get_last("a.key").unwrap();
        assert(block.number, "foo", value.as_str());
        Ok(true)
    }

    fn assert_test_store_set_if_not_exists_string_deltas(block: Block, s: StoreGetString, deltas: Deltas<DeltaString>) -> Result<bool, Error> {
        match block.number {
            1 => {
                if deltas.deltas.len() != 1 {
                    panic!("expected 1 delta, got {}", deltas.deltas.len());
                }

                let delta_0 = deltas.deltas.get(0).unwrap();
                assert(block.number, "", delta_0.old_value.as_str());
                assert(block.number, "foo", delta_0.new_value.as_str());
            },
            _ => {
                if deltas.deltas.len() != 0 {
                    panic!("expected 0 delta, got {}", deltas.deltas.len());
                }
            }
        }

        Ok(true)
    }

    fn assert_all_test_delete_prefix(assert_test_store_delete_prefix: bool, store: substreams::store::StoreSetInt64) {
        //
    }

    fn assert_all_test_i64(assert_test_store_add_i64: bool, assert_test_store_add_i64_deltas: bool, assert_test_store_set_i64: bool, assert_test_store_set_i64_deltas: bool, assert_test_store_set_if_not_exists_i64: bool, assert_test_store_set_if_not_exists_i64_deltas: bool, assert_test_store_min_i64: bool, assert_test_store_min_i64_deltas: bool, assert_test_store_max_i64: bool, assert_test_store_max_i64_deltas: bool, store: substreams::store::StoreSetInt64) {
        //
    }

    fn assert_all_test_float64(assert_test_store_add_float64: bool, assert_test_store_add_float64_deltas: bool, assert_test_store_set_float64: bool, assert_test_store_set_float64_deltas: bool, assert_test_store_set_if_not_exists_float64: bool, assert_test_store_set_if_not_exists_float64_deltas: bool, assert_test_store_min_float64: bool, assert_test_store_min_float64_deltas: bool, assert_test_store_max_float64: bool, assert_test_store_max_float64_deltas: bool, store: StoreSetInt64) {
        //
    }

    fn assert_all_test_bigint(assert_test_store_add_bigint: bool, assert_test_store_add_bigint_deltas: bool, assert_test_store_set_bigint: bool, assert_test_store_set_bigint_deltas: bool, assert_test_store_set_if_not_exists_bigint: bool, assert_test_store_set_if_not_exists_bigint_deltas: bool, assert_test_store_min_bigint: bool, assert_test_store_min_bigint_deltas: bool, assert_test_store_max_bigint: bool, assert_test_store_max_bigint_deltas: bool, store: StoreSetInt64) {
        //
    }

    fn assert_all_test_bigdecimal(assert_test_store_add_bigdecimal: bool, assert_test_store_add_bigdecimal_deltas: bool, assert_test_store_set_bigdecimal: bool, assert_test_store_set_bigdecimal_deltas: bool, assert_test_store_set_if_not_exists_bigdecimal: bool, assert_test_store_set_if_not_exists_bigdecimal_deltas: bool, assert_test_store_min_bigdecimal: bool, assert_test_store_min_bigdecimal_deltas: bool, assert_test_store_max_bigdecimal: bool, assert_test_store_max_bigdecimal_deltas: bool, store: StoreSetInt64) {
        //
    }

    fn assert_all_test_string(assert_test_store_append_string: bool, assert_test_store_append_string_deltas: bool, assert_test_store_set_string: bool, assert_test_store_set_string_deltas: bool, assert_test_store_set_if_not_exists_string: bool, assert_test_store_set_if_not_exists_string_deltas: bool, store: StoreSetInt64) {
        //
    }

    fn assert_all_test(assert_all_test_delete_prefix: StoreGetInt64, assert_all_test_string: StoreGetInt64, assert_all_test_i64: StoreGetInt64, assert_all_test_float64: StoreGetInt64, assert_all_test_bigint: StoreGetInt64, assert_all_test_bigdecimal: StoreGetInt64, store: StoreSetInt64) {
        //
    }

    fn store_root(block: test::Block, store: StoreSetInt64) {
        store.set(block.number, format!("key.{}", block.number), &(block.number as i64));
    }

    fn store_depend(block: test::Block, store_root: StoreGetInt64, _store: StoreSetInt64) {
        let value = store_root.get_last("key.3");
        assert(block.number, true, value.is_some())
    }

    fn store_depends_on_depend(block: test::Block, store_root: StoreGetInt64, _store_depend: StoreGetInt64, _store: StoreSetInt64) {
        let value = store_root.get_last("key.3");
        assert(block.number, true, value.is_some())
    }
}


fn expected_operation(block_num: u64) -> substreams::pb::substreams::store_delta::Operation {
    let mut op = substreams::pb::substreams::store_delta::Operation::Update;
    if block_num == 1 {
        op = substreams::pb::substreams::store_delta::Operation::Create;
    }
    op
}

fn assert<T: Debug + Display + PartialEq>(block_number: u64, expected_value: T, actual_value: T) {
    assert_eq!(
        expected_value, actual_value,
        "expected {} got {} at block {}",
        expected_value, actual_value, block_number
    )
}
