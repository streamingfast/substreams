mod generated;

use prost::encoding::float;
use std::borrow::Borrow;
use std::fmt::{Debug, Display};
use std::ops::Deref;
use prost::Message;
use substreams::errors::Error;
use substreams::prelude::*;

use substreams::scalar::BigDecimal;
use substreams::store::{
    DeltaBigDecimal, DeltaFloat64, StoreAddBigDecimal, StoreAddFloat64, StoreGetBigDecimal,
    StoreGetFloat64, StoreMaxBigDecimal, StoreMaxBigInt, StoreMaxFloat64, StoreMaxInt64,
    StoreMinBigDecimal, StoreMinBigInt, StoreMinFloat64, StoreSetBigDecimal, StoreSetBigInt,
    StoreSetFloat64,
};
use substreams::{errors, log_info, scalar::BigInt, store::{
    DeltaBigInt, DeltaInt64, Deltas, StoreAdd, StoreAddBigInt, StoreAddInt64, StoreDelete,
    StoreGet, StoreGetBigInt, StoreGetInt64, StoreMinInt64, StoreNew, StoreSet,
    StoreSetIfNotExists, StoreSetIfNotExistsInt64, StoreSetIfNotExistsProto, StoreSetInt64,
    StoreSetProto,
}};
use substreams::pb::substreams::store_delta::Operation;
use substreams::pb::substreams::store_delta::Operation::{Create, Update};

use crate::pb::test;
use crate::pb::test::Block;

mod pb;

const TO_SET: i64 = 100;
const TO_ADD: i64 = 1;
const TO_SUBTRACT: i64 = -1;

impl generated::substreams::SubstreamsTrait for generated::substreams::Substreams {
    fn test_map(params: String, blk: test::Block) -> Result<test::MapResult, errors::Error> {
        let out = test::MapResult {
            block_number: blk.number,
            block_hash: blk.id,
        };

        if params != "" {
            assert_eq!(params, "my test params");
        }

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

    fn assert_test_store_delete_prefix(
        block: test::Block,
        s: StoreGetInt64,
    ) -> Result<test::Boolean, errors::Error> {
        let to_read_key = format!("key:{}", block.number);
        assert_eq!(TO_SET, s.get_last(to_read_key).unwrap());

        if block.number > 1 {
            let previous_block_num = block.number - 1;
            let deleted_key = format!("key:{}", previous_block_num);
            assert_eq!(None, s.get_last(deleted_key))
        }

        Ok(test::Boolean { result: true })
    }

    ////////////////////// INT 64 //////////////////////

    fn setup_test_store_add_i64(block: test::Block, s: StoreAddInt64) {
        s.add(block.number, "a.key", i64::MAX);
        s.add(block.number, "a.key", i64::MIN);
        s.add(block.number, "a.key", 1);
    }

    fn assert_test_store_add_i64(
        block: test::Block,
        s: StoreGetInt64,
    ) -> Result<test::Boolean, errors::Error> {
        assert(block.number, true, s.has_last("a.key"));
        assert(block.number, false, s.has_last("b.key"));;

        assert(block.number, 0, s.get_last("a.key").unwrap());
        Ok(test::Boolean { result: true })
    }

    fn assert_test_store_add_i64_deltas(
        block: test::Block,
        _store: StoreGetInt64,
        deltas: Deltas<DeltaInt64>,
    ) -> Result<test::Boolean, errors::Error> {
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

        Ok(test::Boolean { result: true })
    }

    fn setup_test_store_set_i64(block: test::Block, store: StoreSetInt64) {
        store.set(block.number, "0", &0);
        store.set(block.number, "min", &i64::MIN);
        store.set(block.number, "max", &i64::MAX);
    }

    fn assert_test_store_set_i64(
        block: test::Block,
        s: StoreGetInt64,
    ) -> Result<test::Boolean, errors::Error> {
        assert(block.number, 0, s.get_last("0").unwrap());
        assert(block.number, i64::MIN, s.get_last("min").unwrap());
        assert(block.number, i64::MAX, s.get_last("max").unwrap());
        Ok(test::Boolean { result: true })
    }

    fn assert_test_store_set_i64_deltas(
        block: test::Block,
        s: StoreGetInt64,
        deltas: Deltas<DeltaInt64>,
    ) -> Result<test::Boolean, errors::Error> {
        if deltas.deltas.len() != 3 {
            panic!("expected 3 deltas, got {}", deltas.deltas.len());
        }

        let delta_0 = deltas.deltas.get(0).unwrap();
        assert(block.number, 0, delta_0.new_value);
        let delta_1 = deltas.deltas.get(1).unwrap();
        assert(block.number, i64::MIN, delta_1.new_value);
        let delta_2 = deltas.deltas.get(2).unwrap();
        assert(block.number, i64::MAX, delta_2.new_value);

        Ok(test::Boolean { result: true })
    }

    fn setup_test_store_set_if_not_exists_i64(block: Block, s: StoreSetIfNotExistsInt64) {
        s.set_if_not_exists(block.number, "key.0", &10);
        s.set_if_not_exists(block.number, "key.0", &1000);
    }

    fn assert_test_store_set_if_not_exists_i64(
        block: Block,
        s: StoreGetInt64,
    ) -> Result<test::Boolean, Error> {
        assert(block.number, 10, s.get_last("key.0").unwrap());
        Ok(test::Boolean { result: true })
    }

    fn assert_test_store_set_if_not_exists_i64_deltas(
        block: Block,
        s: StoreGetInt64,
        deltas: Deltas<DeltaInt64>,
    ) -> Result<test::Boolean, Error> {
        match block.number {
            1 => {
                if deltas.deltas.len() != 1 {
                    panic!("expected 1 delta, got {}", deltas.deltas.len());
                }

                let delta_0 = deltas.deltas.get(0).unwrap();
                assert(block.number, 0, delta_0.old_value);
                assert(block.number, 10, delta_0.new_value);
            }
            _ => {
                if deltas.deltas.len() != 0 {
                    panic!("expected 0 deltas, got {}", deltas.deltas.len());
                }
            }
        }

        Ok(test::Boolean { result: true })
    }

    fn setup_test_store_min_i64(block: test::Block, s: StoreMinInt64) {
        match block.number {
            1 => {
                s.min(block.number, "a.key", i64::MIN);
                s.min(block.number, "a.key", i64::MAX);
            }
            _ => {
                s.min(block.number, "a.key", i64::MIN);
            }
        }
    }

    fn assert_test_store_min_i64(
        block: test::Block,
        s: StoreGetInt64,
    ) -> Result<test::Boolean, errors::Error> {
        assert(block.number, i64::MIN, s.get_last("a.key").unwrap());
        Ok(test::Boolean { result: true })
    }

    fn assert_test_store_min_i64_deltas(
        block: test::Block,
        _store: StoreGetInt64,
        deltas: Deltas<DeltaInt64>,
    ) -> Result<test::Boolean, errors::Error> {
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
            }
            _ => {
                if deltas.deltas.len() != 1 {
                    panic!("expected 1 delta, got {}", deltas.deltas.len());
                }

                let delta_0 = deltas.deltas.get(0).unwrap();
                assert(1, i64::MIN, delta_0.old_value);
                assert(1, i64::MIN, delta_0.new_value);
            }
        }

        Ok(test::Boolean { result: true })
    }

    fn setup_test_store_max_i64(block: Block, s: StoreMaxInt64) {
        match block.number {
            1 => {
                s.max(block.number, "a.key", i64::MAX);
                s.max(block.number, "a.key", i64::MIN);
            }
            _ => {
                s.max(block.number, "a.key", i64::MAX);
            }
        }
    }

    fn assert_test_store_max_i64(
        block: Block,
        s: substreams::store::StoreGetInt64,
    ) -> Result<test::Boolean, Error> {
        assert(block.number, i64::MAX, s.get_last("a.key").unwrap());
        Ok(test::Boolean { result: true })
    }

    fn assert_test_store_max_i64_deltas(
        block: Block,
        _store: substreams::store::StoreGetInt64,
        deltas: Deltas<substreams::store::DeltaInt64>,
    ) -> Result<test::Boolean, Error> {
        match block.number {
            1 => {
                if deltas.deltas.len() != 2 {
                    panic!("expected 2 deltas, got {}", deltas.deltas.len());
                }

                let delta_0 = deltas.deltas.get(0).unwrap();
                assert(block.number, 0, delta_0.old_value);
                assert(block.number, i64::MAX, delta_0.new_value);
                match delta_0.operation {
                    Create => {}
                    _ => {
                        panic!("expected Create, got {:?}", delta_0.operation);
                    }
                }

                let delta_1 = deltas.deltas.get(1).unwrap();
                assert(block.number, i64::MAX, delta_1.old_value);
                assert(block.number, i64::MAX, delta_1.new_value);
                match delta_1.operation {
                    Update => {}
                    _ => {
                        panic!("expected Update, got {:?}", delta_1.operation);
                    }
                }
            }
            _ => {
                if deltas.deltas.len() != 1 {
                    panic!("expected 1 delta, got {}", deltas.deltas.len());
                }

                let delta_0 = deltas.deltas.get(0).unwrap();
                assert(block.number, i64::MAX, delta_0.old_value);
                assert(block.number, i64::MAX, delta_0.new_value);
                match delta_0.operation {
                    Update => {}
                    _ => {
                        panic!("expected Update, got {:?}", delta_0.operation);
                    }
                }
            }
        }

        Ok(test::Boolean { result: true })
    }

    ////////////////////// FLOAT 64 //////////////////////

    fn setup_test_store_add_float64(block: Block, s: StoreAddFloat64) {
        s.add(block.number, "a.key", 1.0);
        s.add(block.number, "a.key", 0.0);
        s.add(block.number, "a.key", -1.0);
    }

    fn assert_test_store_add_float64(
        block: Block,
        s: StoreGetFloat64,
    ) -> Result<test::Boolean, Error> {
        let value = s.get_last("a.key").unwrap();
        assert(block.number, 0.0, value);
        Ok(test::Boolean { result: true })
    }

    fn assert_test_store_add_float64_deltas(
        block: Block,
        s: StoreGetFloat64,
        deltas: Deltas<DeltaFloat64>,
    ) -> Result<test::Boolean, Error> {
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

        Ok(test::Boolean { result: true })
    }

    fn setup_test_store_set_float64(block: Block, store: StoreSetFloat64) {
        store.set(block.number, "0", &0.0);
        store.set(block.number, "min", &f64::MIN);
        store.set(block.number, "max", &f64::MAX);
    }

    fn assert_test_store_set_float64(
        block: Block,
        s: StoreGetFloat64,
    ) -> Result<test::Boolean, Error> {
        assert(block.number, 0.0, s.get_last("0").unwrap());
        assert(block.number, f64::MIN, s.get_last("min").unwrap());
        assert(block.number, f64::MAX, s.get_last("max").unwrap());
        Ok(test::Boolean { result: true })
    }

    fn assert_test_store_set_float64_deltas(
        block: Block,
        setup_test_store_set_float64: StoreGetFloat64,
        deltas: Deltas<DeltaFloat64>,
    ) -> Result<test::Boolean, Error> {
        if deltas.deltas.len() != 3 {
            panic!("expected 3 deltas, got {}", deltas.deltas.len());
        }

        let delta_0 = deltas.deltas.get(0).unwrap();
        assert(block.number, 0.0, delta_0.new_value);
        let delta_1 = deltas.deltas.get(1).unwrap();
        assert(block.number, f64::MIN, delta_1.new_value);
        let delta_2 = deltas.deltas.get(2).unwrap();
        assert(block.number, f64::MAX, delta_2.new_value);

        Ok(test::Boolean { result: true })
    }

    fn setup_test_store_set_if_not_exists_float64(block: Block, s: StoreSetIfNotExistsFloat64) {
        s.set_if_not_exists(block.number, "key.0", &10.0);
        s.set_if_not_exists(block.number, "key.0", &1000.0);
    }

    fn assert_test_store_set_if_not_exists_float64(
        block: Block,
        s: StoreGetFloat64,
    ) -> Result<test::Boolean, Error> {
        assert(block.number, 10.0, s.get_last("key.0").unwrap());
        Ok(test::Boolean { result: true })
    }

    fn assert_test_store_set_if_not_exists_float64_deltas(
        block: Block,
        s: StoreGetFloat64,
        deltas: Deltas<DeltaFloat64>,
    ) -> Result<test::Boolean, Error> {
        match block.number {
            1 => {
                if deltas.deltas.len() != 1 {
                    panic!("expected 1 delta, got {}", deltas.deltas.len());
                }

                let delta_0 = deltas.deltas.get(0).unwrap();
                assert(block.number, 0.0, delta_0.old_value);
                assert(block.number, 10.0, delta_0.new_value);
            }
            _ => {
                if deltas.deltas.len() != 0 {
                    panic!("expected 0 deltas, got {}", deltas.deltas.len());
                }
            }
        }

        Ok(test::Boolean { result: true })
    }

    fn setup_test_store_min_float64(block: Block, s: StoreMinFloat64) {
        match block.number {
            1 => {
                s.min(block.number, "a.key", f64::MIN);
                s.min(block.number, "a.key", f64::MAX);
            }
            _ => {
                s.min(block.number, "a.key", f64::MIN);
            }
        }
    }

    fn assert_test_store_min_float64(
        block: Block,
        s: StoreGetFloat64,
    ) -> Result<test::Boolean, Error> {
        assert(block.number, f64::MIN, s.get_last("a.key").unwrap());
        Ok(test::Boolean { result: true })
    }

    fn assert_test_store_min_float64_deltas(
        block: Block,
        s: StoreGetFloat64,
        deltas: Deltas<DeltaFloat64>,
    ) -> Result<test::Boolean, Error> {
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
            }
            _ => {
                if deltas.deltas.len() != 1 {
                    panic!("expected 1 delta, got {}", deltas.deltas.len());
                }

                let delta_0 = deltas.deltas.get(0).unwrap();
                assert(block.number, f64::MIN, delta_0.old_value);
                assert(block.number, f64::MIN, delta_0.new_value);
            }
        }

        Ok(test::Boolean { result: true })
    }

    fn setup_test_store_max_float64(block: Block, s: StoreMaxFloat64) {
        match block.number {
            1 => {
                s.max(block.number, "a.key", f64::MAX);
                s.max(block.number, "a.key", f64::MIN);
            }
            _ => {
                s.max(block.number, "a.key", f64::MAX);
            }
        }
    }

    fn assert_test_store_max_float64(
        block: Block,
        s: StoreGetFloat64,
    ) -> Result<test::Boolean, Error> {
        assert(block.number, f64::MAX, s.get_last("a.key").unwrap());
        Ok(test::Boolean { result: true })
    }

    fn assert_test_store_max_float64_deltas(
        block: Block,
        s: StoreGetFloat64,
        deltas: Deltas<DeltaFloat64>,
    ) -> Result<test::Boolean, Error> {
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
            }
            _ => {
                if deltas.deltas.len() != 1 {
                    panic!("expected 1 delta, got {}", deltas.deltas.len());
                }

                let delta_0 = deltas.deltas.get(0).unwrap();
                assert(block.number, f64::MAX, delta_0.old_value);
                assert(block.number, f64::MAX, delta_0.new_value);
            }
        }

        Ok(test::Boolean { result: true })
    }

    ////////////////////// BIG INT //////////////////////

    fn setup_test_store_add_bigint(block: Block, s: StoreAddBigInt) {
        s.add(block.number, "a.key", BigInt::from(1));
        s.add(block.number, "a.key", BigInt::from(0));
        s.add(block.number, "a.key", BigInt::from(-1));

        s.add(block.number, "a.key.pos", BigInt::from(1));
        s.add(block.number, "a.key.neg", BigInt::from(-1));
    }

    fn assert_test_store_add_bigint(
        block: Block,
        s: StoreGetBigInt,
    ) -> Result<test::Boolean, Error> {
        let value = s.get_last("a.key").unwrap();
        assert(block.number, BigInt::from(0), value);
        Ok(test::Boolean { result: true })
    }

    fn assert_test_store_add_bigint_deltas(
        block: Block,
        s: StoreGetBigInt,
        deltas: Deltas<DeltaBigInt>,
    ) -> Result<test::Boolean, Error> {
        if deltas.deltas.len() != 5 {
            panic!("expected 5 deltas, got {}", deltas.deltas.len());
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

        Ok(test::Boolean { result: true })
    }

    fn setup_test_store_set_bigint(block: Block, store: StoreSetBigInt) {
        store.set(block.number, "0", &BigInt::from(0));
        store.set(block.number, "min", &BigInt::from(i64::MIN));
        store.set(block.number, "max", &BigInt::from(i64::MAX));
    }

    fn assert_test_store_set_bigint(
        block: Block,
        s: StoreGetBigInt,
    ) -> Result<test::Boolean, Error> {
        assert(
            block.number,
            &BigInt::from(i64::from(0)),
            s.get_last("0").unwrap().borrow(),
        );
        assert(
            block.number,
            &BigInt::from(i64::MIN),
            s.get_last("min").unwrap().borrow(),
        );
        assert(
            block.number,
            &BigInt::from(i64::MAX),
            s.get_last("max").unwrap().borrow(),
        );
        Ok(test::Boolean { result: true })
    }

    fn assert_test_store_set_bigint_deltas(
        block: Block,
        s: StoreGetBigInt,
        deltas: Deltas<DeltaBigInt>,
    ) -> Result<test::Boolean, Error> {
        if deltas.deltas.len() != 3 {
            panic!("expected 3 deltas, got {}", deltas.deltas.len());
        }

        let delta_0 = deltas.deltas.get(0).unwrap();
        assert(
            block.number,
            &BigInt::from(i64::from(0)),
            delta_0.new_value.borrow(),
        );
        let delta_1 = deltas.deltas.get(1).unwrap();
        assert(
            block.number,
            &BigInt::from(i64::MIN),
            delta_1.new_value.borrow(),
        );
        let delta_2 = deltas.deltas.get(2).unwrap();
        assert(
            block.number,
            &BigInt::from(i64::MAX),
            delta_2.new_value.borrow(),
        );

        Ok(test::Boolean { result: true })
    }

    fn setup_test_store_set_if_not_exists_bigint(block: Block, store: StoreSetIfNotExistsBigInt) {
        store.set_if_not_exists(block.number, "key.a", &BigInt::from(10));
        store.set_if_not_exists(block.number, "key.a", &BigInt::from(1000));
    }

    fn assert_test_store_set_if_not_exists_bigint(
        block: Block,
        s: StoreGetBigInt,
    ) -> Result<test::Boolean, Error> {
        assert(
            block.number,
            &BigInt::from(i64::from(10)),
            s.get_last("key.a").unwrap().borrow(),
        );
        Ok(test::Boolean { result: true })
    }

    fn assert_test_store_set_if_not_exists_bigint_deltas(
        block: Block,
        s: StoreGetBigInt,
        deltas: Deltas<DeltaBigInt>,
    ) -> Result<test::Boolean, Error> {
        match block.number {
            1 => {
                if deltas.deltas.len() != 1 {
                    panic!("expected 1 delta, got {}", deltas.deltas.len());
                }

                let delta_0 = deltas.deltas.get(0).unwrap();
                assert(
                    block.number,
                    &BigInt::from(i64::from(0)),
                    delta_0.old_value.borrow(),
                );
                assert(
                    block.number,
                    &BigInt::from(i64::from(10)),
                    delta_0.new_value.borrow(),
                );
            }
            _ => {
                if deltas.deltas.len() != 0 {
                    panic!("expected 0 deltas, got {}", deltas.deltas.len());
                }
            }
        }

        Ok(test::Boolean { result: true })
    }

    fn setup_test_store_min_bigint(block: Block, s: StoreMinBigInt) {
        match block.number {
            1 => {
                s.min(block.number, "a.key", BigInt::from(-1));
                s.min(block.number, "a.key", BigInt::from(1));
            }
            _ => {
                s.min(block.number, "a.key", BigInt::from(-1));
            }
        }
    }

    fn assert_test_store_min_bigint(
        block: Block,
        s: StoreGetBigInt,
    ) -> Result<test::Boolean, Error> {
        assert(block.number, BigInt::from(-1), s.get_last("a.key").unwrap());
        Ok(test::Boolean { result: true })
    }

    fn assert_test_store_min_bigint_deltas(
        block: Block,
        s: StoreGetBigInt,
        deltas: Deltas<DeltaBigInt>,
    ) -> Result<test::Boolean, Error> {
        match block.number {
            1 => {
                if deltas.deltas.len() != 2 {
                    panic!("expected 2 deltas, got {}", deltas.deltas.len());
                }

                let delta_0 = deltas.deltas.get(0).unwrap();
                assert(
                    block.number,
                    &BigInt::from(i64::from(0)),
                    delta_0.old_value.borrow(),
                );
                assert(
                    block.number,
                    &BigInt::from(i64::from(-1)),
                    delta_0.new_value.borrow(),
                );

                let delta_1 = deltas.deltas.get(1).unwrap();
                assert(
                    block.number,
                    &BigInt::from(i64::from(-1)),
                    delta_1.old_value.borrow(),
                );
                assert(
                    block.number,
                    &BigInt::from(i64::from(-1)),
                    delta_1.new_value.borrow(),
                );
            }
            _ => {
                if deltas.deltas.len() != 1 {
                    panic!("expected 1 delta, got {}", deltas.deltas.len());
                }

                let delta_0 = deltas.deltas.get(0).unwrap();
                assert(
                    block.number,
                    &BigInt::from(i64::from(-1)),
                    delta_0.old_value.borrow(),
                );
                assert(
                    block.number,
                    &BigInt::from(i64::from(-1)),
                    delta_0.new_value.borrow(),
                );
            }
        }

        Ok(test::Boolean { result: true })
    }

    fn setup_test_store_max_bigint(block: Block, s: StoreMaxBigInt) {
        match block.number {
            1 => {
                s.max(block.number, "a.key", BigInt::from(1));
                s.max(block.number, "a.key", BigInt::from(-1));
            }
            _ => {
                s.max(block.number, "a.key", BigInt::from(1));
            }
        }
    }

    fn assert_test_store_max_bigint(
        block: Block,
        s: StoreGetBigInt,
    ) -> Result<test::Boolean, Error> {
        assert(block.number, BigInt::from(1), s.get_last("a.key").unwrap());
        Ok(test::Boolean { result: true })
    }

    fn assert_test_store_max_bigint_deltas(
        block: Block,
        s: StoreGetBigInt,
        deltas: Deltas<DeltaBigInt>,
    ) -> Result<test::Boolean, Error> {
        match block.number {
            1 => {
                if deltas.deltas.len() != 2 {
                    panic!("expected 2 deltas, got {}", deltas.deltas.len());
                }

                let delta_0 = deltas.deltas.get(0).unwrap();
                assert(
                    block.number,
                    &BigInt::from(i64::from(0)),
                    delta_0.old_value.borrow(),
                );
                assert(
                    block.number,
                    &BigInt::from(i64::from(1)),
                    delta_0.new_value.borrow(),
                );

                let delta_1 = deltas.deltas.get(1).unwrap();
                assert(
                    block.number,
                    &BigInt::from(i64::from(1)),
                    delta_1.old_value.borrow(),
                );
                assert(
                    block.number,
                    &BigInt::from(i64::from(1)),
                    delta_1.new_value.borrow(),
                );
            }
            _ => {
                if deltas.deltas.len() != 1 {
                    panic!("expected 1 delta, got {}", deltas.deltas.len());
                }

                let delta_0 = deltas.deltas.get(0).unwrap();
                assert(
                    block.number,
                    &BigInt::from(i64::from(1)),
                    delta_0.old_value.borrow(),
                );
                assert(
                    block.number,
                    &BigInt::from(i64::from(1)),
                    delta_0.new_value.borrow(),
                );
            }
        }

        Ok(test::Boolean { result: true })
    }

    ////////////////////// BIG DECIMAL //////////////////////

    fn setup_test_store_add_bigdecimal(block: Block, s: StoreAddBigDecimal) {
        s.add(block.number, "a.key", BigDecimal::from(1));
        s.add(block.number, "a.key", BigDecimal::from(0));
        s.add(block.number, "a.key", BigDecimal::from(-1));
    }

    fn assert_test_store_add_bigdecimal(
        block: Block,
        s: StoreGetBigDecimal,
    ) -> Result<test::Boolean, Error> {
        let value = s.get_last("a.key").unwrap();
        assert(block.number, BigDecimal::from(0), value);
        Ok(test::Boolean { result: true })
    }

    fn assert_test_store_add_bigdecimal_deltas(
        block: Block,
        s: StoreGetBigDecimal,
        deltas: Deltas<DeltaBigDecimal>,
    ) -> Result<test::Boolean, Error> {
        if deltas.deltas.len() != 3 {
            panic!("expected 3 deltas, got {}", deltas.deltas.len());
        }

        let delta_0 = deltas.deltas.get(0).unwrap();
        assert(
            block.number,
            &BigDecimal::from(0),
            delta_0.old_value.borrow(),
        );
        assert(
            block.number,
            &BigDecimal::from(1),
            delta_0.new_value.borrow(),
        );

        let delta_1 = deltas.deltas.get(1).unwrap();
        assert(
            block.number,
            &BigDecimal::from(1),
            delta_1.old_value.borrow(),
        );
        assert(
            block.number,
            &BigDecimal::from(1),
            delta_1.new_value.borrow(),
        );

        let delta_2 = deltas.deltas.get(2).unwrap();
        assert(
            block.number,
            &BigDecimal::from(1),
            delta_2.old_value.borrow(),
        );
        assert(
            block.number,
            &BigDecimal::from(0),
            delta_2.new_value.borrow(),
        );

        Ok(test::Boolean { result: true })
    }

    fn setup_test_store_set_bigdecimal(block: Block, s: StoreSetBigDecimal) {
        s.set(block.number, "0", &BigDecimal::from(0));
        s.set(block.number, "min", &BigDecimal::from(i64::MIN));
        s.set(block.number, "max", &BigDecimal::from(i64::MAX));
    }

    fn assert_test_store_set_bigdecimal(
        block: Block,
        s: StoreGetBigDecimal,
    ) -> Result<test::Boolean, Error> {
        assert(
            block.number,
            &BigDecimal::from(i64::from(0)),
            s.get_last("0").unwrap().borrow(),
        );
        assert(
            block.number,
            &BigDecimal::from(i64::MIN),
            s.get_last("min").unwrap().borrow(),
        );
        assert(
            block.number,
            &BigDecimal::from(i64::MAX),
            s.get_last("max").unwrap().borrow(),
        );
        Ok(test::Boolean { result: true })
    }

    fn assert_test_store_set_bigdecimal_deltas(
        block: Block,
        s: StoreGetBigDecimal,
        deltas: Deltas<DeltaBigDecimal>,
    ) -> Result<test::Boolean, Error> {
        if deltas.deltas.len() != 3 {
            panic!("expected 3 deltas, got {}", deltas.deltas.len());
        }

        let delta_0 = deltas.deltas.get(0).unwrap();
        assert(
            block.number,
            &BigDecimal::from(i64::from(0)),
            delta_0.new_value.borrow(),
        );
        let delta_1 = deltas.deltas.get(1).unwrap();
        assert(
            block.number,
            &BigDecimal::from(i64::MIN),
            delta_1.new_value.borrow(),
        );
        let delta_2 = deltas.deltas.get(2).unwrap();
        assert(
            block.number,
            &BigDecimal::from(i64::MAX),
            delta_2.new_value.borrow(),
        );

        Ok(test::Boolean { result: true })
    }

    fn setup_test_store_set_if_not_exists_bigdecimal(
        block: Block,
        store: StoreSetIfNotExistsBigDecimal,
    ) {
        store.set_if_not_exists(block.number, "key.a", &BigDecimal::from(10));
        store.set_if_not_exists(block.number, "key.a", &BigDecimal::from(1000));
    }

    fn assert_test_store_set_if_not_exists_bigdecimal(
        block: Block,
        s: StoreGetBigDecimal,
    ) -> Result<test::Boolean, Error> {
        assert(
            block.number,
            &BigDecimal::from(i64::from(10)),
            s.get_last("key.a").unwrap().borrow(),
        );
        Ok(test::Boolean { result: true })
    }

    fn assert_test_store_set_if_not_exists_bigdecimal_deltas(
        block: Block,
        s: StoreGetBigDecimal,
        deltas: Deltas<DeltaBigDecimal>,
    ) -> Result<test::Boolean, Error> {
        match block.number {
            1 => {
                if deltas.deltas.len() != 1 {
                    panic!("expected 1 delta, got {}", deltas.deltas.len());
                }

                let delta_0 = deltas.deltas.get(0).unwrap();
                assert(
                    block.number,
                    &BigDecimal::from(i64::from(0)),
                    delta_0.old_value.borrow(),
                );
                assert(
                    block.number,
                    &BigDecimal::from(i64::from(10)),
                    delta_0.new_value.borrow(),
                );
            }
            _ => {
                if deltas.deltas.len() != 0 {
                    panic!("expected 0 deltas, got {}", deltas.deltas.len());
                }
            }
        }

        Ok(test::Boolean { result: true })
    }

    fn setup_test_store_min_bigdecimal(block: Block, s: StoreMinBigDecimal) {
        match block.number {
            1 => {
                s.min(block.number, "a.key", BigDecimal::from(-1));
                s.min(block.number, "a.key", BigDecimal::from(1));
            }
            _ => {
                s.min(block.number, "a.key", BigDecimal::from(-1));
            }
        }
    }

    fn assert_test_store_min_bigdecimal(
        block: Block,
        s: StoreGetBigDecimal,
    ) -> Result<test::Boolean, Error> {
        assert(
            block.number,
            BigDecimal::from(-1),
            s.get_last("a.key").unwrap(),
        );
        Ok(test::Boolean { result: true })
    }

    fn assert_test_store_min_bigdecimal_deltas(
        block: Block,
        s: StoreGetBigDecimal,
        deltas: Deltas<DeltaBigDecimal>,
    ) -> Result<test::Boolean, Error> {
        match block.number {
            1 => {
                if deltas.deltas.len() != 2 {
                    panic!("expected 2 deltas, got {}", deltas.deltas.len());
                }

                let delta_0 = deltas.deltas.get(0).unwrap();
                assert(
                    block.number,
                    &BigDecimal::from(i64::from(0)),
                    delta_0.old_value.borrow(),
                );
                assert(
                    block.number,
                    &BigDecimal::from(i64::from(-1)),
                    delta_0.new_value.borrow(),
                );

                let delta_1 = deltas.deltas.get(1).unwrap();
                assert(
                    block.number,
                    &BigDecimal::from(i64::from(-1)),
                    delta_1.old_value.borrow(),
                );
                assert(
                    block.number,
                    &BigDecimal::from(i64::from(-1)),
                    delta_1.new_value.borrow(),
                );
            }
            _ => {
                if deltas.deltas.len() != 1 {
                    panic!("expected 1 delta, got {}", deltas.deltas.len());
                }

                let delta_0 = deltas.deltas.get(0).unwrap();
                assert(
                    block.number,
                    &BigDecimal::from(i64::from(-1)),
                    delta_0.old_value.borrow(),
                );
                assert(
                    block.number,
                    &BigDecimal::from(i64::from(-1)),
                    delta_0.new_value.borrow(),
                );
            }
        }

        Ok(test::Boolean { result: true })
    }

    fn setup_test_store_max_bigdecimal(block: Block, s: StoreMaxBigDecimal) {
        match block.number {
            1 => {
                s.max(block.number, "a.key", BigDecimal::from(1));
                s.max(block.number, "a.key", BigDecimal::from(-1));
            }
            _ => {
                s.max(block.number, "a.key", BigDecimal::from(1));
            }
        }
    }

    fn assert_test_store_max_bigdecimal(
        block: Block,
        s: StoreGetBigDecimal,
    ) -> Result<test::Boolean, Error> {
        assert(
            block.number,
            BigDecimal::from(1),
            s.get_last("a.key").unwrap(),
        );
        Ok(test::Boolean { result: true })
    }

    fn assert_test_store_max_bigdecimal_deltas(
        block: Block,
        s: StoreGetBigDecimal,
        deltas: Deltas<DeltaBigDecimal>,
    ) -> Result<test::Boolean, Error> {
        match block.number {
            1 => {
                if deltas.deltas.len() != 2 {
                    panic!("expected 2 deltas, got {}", deltas.deltas.len());
                }

                let delta_0 = deltas.deltas.get(0).unwrap();
                assert(
                    block.number,
                    &BigDecimal::from(i64::from(0)),
                    delta_0.old_value.borrow(),
                );
                assert(
                    block.number,
                    &BigDecimal::from(i64::from(1)),
                    delta_0.new_value.borrow(),
                );

                let delta_1 = deltas.deltas.get(1).unwrap();
                assert(
                    block.number,
                    &BigDecimal::from(i64::from(1)),
                    delta_1.old_value.borrow(),
                );
                assert(
                    block.number,
                    &BigDecimal::from(i64::from(1)),
                    delta_1.new_value.borrow(),
                );
            }
            _ => {
                if deltas.deltas.len() != 1 {
                    panic!("expected 1 delta, got {}", deltas.deltas.len());
                }

                let delta_0 = deltas.deltas.get(0).unwrap();
                assert(
                    block.number,
                    &BigDecimal::from(i64::from(1)),
                    delta_0.old_value.borrow(),
                );
                assert(
                    block.number,
                    &BigDecimal::from(i64::from(1)),
                    delta_0.new_value.borrow(),
                );
            }
        }

        Ok(test::Boolean { result: true })
    }

    ////////////////////// STRING //////////////////////

    fn setup_test_store_set_string(block: Block, store: StoreSetString) {
        store.set(block.number, "a.key", &"foo".to_string());
    }

    fn assert_test_store_set_string(
        block: Block,
        store: StoreGetString,
    ) -> Result<test::Boolean, Error> {
        let value = store.get_last("a.key").unwrap();
        assert(block.number, "foo", value.as_str());
        Ok(test::Boolean { result: true })
    }

    fn assert_test_store_set_string_deltas(
        block: Block,
        store: StoreGetString,
        deltas: Deltas<DeltaString>,
    ) -> Result<test::Boolean, Error> {
        match block.number {
            1 => {
                if deltas.deltas.len() != 1 {
                    panic!("expected 1 delta, got {}", deltas.deltas.len());
                }

                let delta_0 = deltas.deltas.get(0).unwrap();
                assert(block.number, "", delta_0.old_value.as_str());
                assert(block.number, "foo", delta_0.new_value.as_str());
            }
            _ => {
                if deltas.deltas.len() != 1 {
                    panic!("expected 1 delta, got {}", deltas.deltas.len());
                }

                let delta_0 = deltas.deltas.get(0).unwrap();
                assert(block.number, "foo", delta_0.old_value.as_str());
                assert(block.number, "foo", delta_0.new_value.as_str());
            }
        }

        Ok(test::Boolean { result: true })
    }

    fn setup_test_store_set_if_not_exists_string(block: Block, store: StoreSetIfNotExistsString) {
        store.set_if_not_exists(block.number, "a.key", &"foo".to_string());
        store.set_if_not_exists(block.number, "a.key", &"bar".to_string());
    }

    fn assert_test_store_set_if_not_exists_string(
        block: Block,
        store: StoreGetString,
    ) -> Result<test::Boolean, Error> {
        let value = store.get_last("a.key").unwrap();
        assert(block.number, "foo", value.as_str());
        Ok(test::Boolean { result: true })
    }

    fn assert_test_store_set_if_not_exists_string_deltas(
        block: Block,
        s: StoreGetString,
        deltas: Deltas<DeltaString>,
    ) -> Result<test::Boolean, Error> {
        match block.number {
            1 => {
                if deltas.deltas.len() != 1 {
                    panic!("expected 1 delta, got {}", deltas.deltas.len());
                }

                let delta_0 = deltas.deltas.get(0).unwrap();
                assert(block.number, "", delta_0.old_value.as_str());
                assert(block.number, "foo", delta_0.new_value.as_str());
            }
            _ => {
                if deltas.deltas.len() != 0 {
                    panic!("expected 0 delta, got {}", deltas.deltas.len());
                }
            }
        }

        Ok(test::Boolean { result: true })
    }

    fn setup_test_store_append_string(block: Block, store: StoreAppend<String>) {
        store.append(block.number, "test.key", "a".to_string());
    }

    fn assert_test_store_append_string(
        block: Block,
        store: StoreGetRaw,
    ) -> Result<test::Boolean, Error> {
        match block.number {
            1 => {
                let raw_value = store.get_last("test.key").unwrap();
                let value = String::from_utf8(raw_value).unwrap();
                assert(block.number, "a;", value.as_str());
            }
            3 => {
                let raw_value = store.get_last("test.key").unwrap();
                let value = String::from_utf8(raw_value).unwrap();
                assert(block.number, "a;a;a;", value.as_str());
            }
            _ => {}
        }

        Ok(test::Boolean { result: true })
    }

    fn assert_test_store_append_string_deltas(
        block: Block,
        s: StoreGetRaw,
        deltas: Deltas<DeltaArray<String>>,
    ) -> Result<test::Boolean, Error> {
        match block.number {
            1 => {
                if deltas.deltas.len() != 1 {
                    panic!("expected 1 delta, got {}", deltas.deltas.len());
                }

                let delta_0 = deltas.deltas.get(0).unwrap();

                let old_value = delta_0.old_value.clone().to_vec().join(";");
                let empty_vec: Vec<String> = vec![];
                let old_expected_value = empty_vec.join(";");
                assert(block.number, old_expected_value, old_value);

                let new_value = delta_0.new_value.clone().to_vec().join(";");
                let new_expected_value = vec!["a"].join(";");
                assert(block.number, new_expected_value, new_value);
            }
            3 => {
                if deltas.deltas.len() != 1 {
                    panic!("expected 1 delta, got {}", deltas.deltas.len());
                }

                let delta_0 = deltas.deltas.get(0).unwrap();

                let old_value = delta_0.old_value.clone().to_vec().join(";");
                let old_expected_value = vec!["a", "a"].join(";");
                assert(block.number, old_expected_value, old_value);

                let new_value = delta_0.new_value.clone().to_vec().join(";");
                let new_expected_value = vec!["a", "a", "a"].join(";");
                assert(block.number, new_expected_value, new_value);
            }
            _ => {}
        }

        Ok(test::Boolean { result: true })
    }

    fn store_root(block: test::Block, store: StoreSetInt64) {
        store.set(
            block.number,
            format!("key.{}", block.number),
            &(block.number as i64),
        );
    }

    fn store_depend(block: test::Block, store_root: StoreGetInt64, _store: StoreSetInt64) {
        let value = store_root.get_last("key.3");
        assert(block.number, true, value.is_some())
    }

    fn store_depends_on_depend(
        block: test::Block,
        store_root: StoreGetInt64,
        _store_depend: StoreGetInt64,
        _store: StoreSetInt64,
    ) {
        let value = store_root.get_last("key.3");
        assert(block.number, true, value.is_some())
    }

    fn assert_all_test_i64(
        assert_test_store_add_i64: pb::test::Boolean,
        assert_test_store_add_i64_deltas: pb::test::Boolean,
        assert_test_store_set_i64: pb::test::Boolean,
        assert_test_store_set_i64_deltas: pb::test::Boolean,
        assert_test_store_set_if_not_exists_i64: pb::test::Boolean,
        assert_test_store_set_if_not_exists_i64_deltas: pb::test::Boolean,
        assert_test_store_min_i64: pb::test::Boolean,
        assert_test_store_min_i64_deltas: pb::test::Boolean,
        assert_test_store_max_i64: pb::test::Boolean,
        assert_test_store_max_i64_deltas: pb::test::Boolean,
        store: substreams::store::StoreSetInt64,
    ) {
        //
    }

    fn assert_all_test_float64(
        assert_test_store_add_float64: pb::test::Boolean,
        assert_test_store_add_float64_deltas: pb::test::Boolean,
        assert_test_store_set_float64: pb::test::Boolean,
        assert_test_store_set_float64_deltas: pb::test::Boolean,
        assert_test_store_set_if_not_exists_float64: pb::test::Boolean,
        assert_test_store_set_if_not_exists_float64_deltas: pb::test::Boolean,
        assert_test_store_min_float64: pb::test::Boolean,
        assert_test_store_min_float64_deltas: pb::test::Boolean,
        assert_test_store_max_float64: pb::test::Boolean,
        assert_test_store_max_float64_deltas: pb::test::Boolean,
        store: substreams::store::StoreSetInt64,
    ) {
        //
    }

    fn assert_all_test_bigint(
        assert_test_store_add_bigint: pb::test::Boolean,
        assert_test_store_add_bigint_deltas: pb::test::Boolean,
        assert_test_store_set_bigint: pb::test::Boolean,
        assert_test_store_set_bigint_deltas: pb::test::Boolean,
        assert_test_store_set_if_not_exists_bigint: pb::test::Boolean,
        assert_test_store_set_if_not_exists_bigint_deltas: pb::test::Boolean,
        assert_test_store_min_bigint: pb::test::Boolean,
        assert_test_store_min_bigint_deltas: pb::test::Boolean,
        assert_test_store_max_bigint: pb::test::Boolean,
        assert_test_store_max_bigint_deltas: pb::test::Boolean,
        store: substreams::store::StoreSetInt64,
    ) {
        //
    }

    fn assert_all_test_bigdecimal(
        assert_test_store_add_bigdecimal: pb::test::Boolean,
        assert_test_store_add_bigdecimal_deltas: pb::test::Boolean,
        assert_test_store_set_bigdecimal: pb::test::Boolean,
        assert_test_store_set_bigdecimal_deltas: pb::test::Boolean,
        assert_test_store_set_if_not_exists_bigdecimal: pb::test::Boolean,
        assert_test_store_set_if_not_exists_bigdecimal_deltas: pb::test::Boolean,
        assert_test_store_min_bigdecimal: pb::test::Boolean,
        assert_test_store_min_bigdecimal_deltas: pb::test::Boolean,
        assert_test_store_max_bigdecimal: pb::test::Boolean,
        assert_test_store_max_bigdecimal_deltas: pb::test::Boolean,
        store: substreams::store::StoreSetInt64,
    ) {
        //
    }

    fn assert_all_test_string(
        assert_test_store_append_string: pb::test::Boolean,
        assert_test_store_append_string_deltas: pb::test::Boolean,
        assert_test_store_set_string: pb::test::Boolean,
        assert_test_store_set_string_deltas: pb::test::Boolean,
        assert_test_store_set_if_not_exists_string: pb::test::Boolean,
        assert_test_store_set_if_not_exists_string_deltas: pb::test::Boolean,
        store: substreams::store::StoreSetInt64,
    ) {
        //
    }

    fn assert_all_test_delete_prefix(
        assert_test_store_delete_prefix: pb::test::Boolean,
        store: substreams::store::StoreSetInt64,
    ) {
        //
    }

    fn assert_all_test(
        assert_all_test_delete_prefix: StoreGetInt64,
        assert_all_test_string: StoreGetInt64,
        assert_all_test_i64: StoreGetInt64,
        assert_all_test_float64: StoreGetInt64,
        assert_all_test_bigint: StoreGetInt64,
        assert_all_test_bigdecimal: StoreGetInt64,
    ) -> Result<test::Boolean, Error> {
        return Ok(test::Boolean { result: true });
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
