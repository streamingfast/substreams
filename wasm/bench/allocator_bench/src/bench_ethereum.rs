use crate::pb::sf::ethereum::r#type::v2::Block;
use crate::pb::{BenchOutput, CallData, LargeOutput, TxData};
use substreams::errors::Error;

/// Benchmark 1: Protobuf decode only
#[no_mangle]
pub extern "C" fn bench_decode_only_ethereum(blk_ptr: *mut u8, blk_len: usize) {
    substreams::register_panic_hook();
    let func = || -> Result<BenchOutput, Error> {
        let blk: Block = substreams::proto::decode_ptr(blk_ptr, blk_len)
            .expect("Unable to decode Block");

        Ok(BenchOutput {
            count: blk.transaction_traces.len() as u64,
            ..Default::default()
        })
    };
    substreams::skip_empty_output();
    let result = func();
    if result.is_err() {
        panic!("{:?}", result.unwrap_err());
    }
    substreams::output(result.expect("already checked"));
}

/// Benchmark 2: Many small allocations
#[no_mangle]
pub extern "C" fn bench_small_allocs_ethereum(blk_ptr: *mut u8, blk_len: usize) {
    substreams::register_panic_hook();
    let func = || -> Result<BenchOutput, Error> {
        let blk: Block = substreams::proto::decode_ptr(blk_ptr, blk_len)
            .expect("Unable to decode Block");

        let mut strings = Vec::with_capacity(1000);

        for trx in &blk.transaction_traces {
            strings.push(format!("tx:{}", hex::encode(&trx.hash)));
            strings.push(format!("from:{}", hex::encode(&trx.from)));
            strings.push(format!("to:{}", hex::encode(&trx.to)));

            for call in &trx.calls {
                strings.push(format!("call:{}", call.index));
                for log in &call.logs {
                    strings.push(format!("log:{}", log.index));
                }
            }
        }

        Ok(BenchOutput {
            count: strings.len() as u64,
            ..Default::default()
        })
    };
    substreams::skip_empty_output();
    let result = func();
    if result.is_err() {
        panic!("{:?}", result.unwrap_err());
    }
    substreams::output(result.expect("already checked"));
}

/// Benchmark 3: Large allocation with realloc
#[no_mangle]
pub extern "C" fn bench_large_allocs_ethereum(blk_ptr: *mut u8, blk_len: usize) {
    substreams::register_panic_hook();
    let func = || -> Result<BenchOutput, Error> {
        let blk: Block = substreams::proto::decode_ptr(blk_ptr, blk_len)
            .expect("Unable to decode Block");

        let mut data = Vec::new();

        for trx in &blk.transaction_traces {
            data.extend_from_slice(&trx.hash);
            data.extend_from_slice(&trx.input);
            for call in &trx.calls {
                data.extend_from_slice(&call.input);
                data.extend_from_slice(&call.return_data);
            }
        }

        Ok(BenchOutput {
            count: data.len() as u64,
            ..Default::default()
        })
    };
    substreams::skip_empty_output();
    let result = func();
    if result.is_err() {
        panic!("{:?}", result.unwrap_err());
    }
    substreams::output(result.expect("already checked"));
}

/// Benchmark 4: Mixed allocation pattern
#[no_mangle]
pub extern "C" fn bench_mixed_ethereum(blk_ptr: *mut u8, blk_len: usize) {
    substreams::register_panic_hook();
    let func = || -> Result<LargeOutput, Error> {
        let blk: Block = substreams::proto::decode_ptr(blk_ptr, blk_len)
            .expect("Unable to decode Block");

        let mut output = LargeOutput::default();

        for trx in &blk.transaction_traces {
            let mut tx_data = TxData {
                hash: hex::encode(&trx.hash),
                from: hex::encode(&trx.from),
                to: hex::encode(&trx.to),
                ..Default::default()
            };

            for call in &trx.calls {
                tx_data.calls.push(CallData {
                    index: call.index as u64,
                    input_size: call.input.len() as u64,
                    return_size: call.return_data.len() as u64,
                });
            }

            output.transactions.push(tx_data);
        }

        Ok(output)
    };
    substreams::skip_empty_output();
    let result = func();
    if result.is_err() {
        panic!("{:?}", result.unwrap_err());
    }
    substreams::output(result.expect("already checked"));
}
