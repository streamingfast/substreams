use crate::pb::sf::solana::r#type::v1::Block;
use crate::pb::{BenchOutput, LargeOutput};
use substreams::errors::Error;

/// Benchmark 1: Protobuf decode only
#[no_mangle]
pub extern "C" fn bench_decode_only_solana(blk_ptr: *mut u8, blk_len: usize) {
    substreams::register_panic_hook();
    let func = || -> Result<BenchOutput, Error> {
        let blk: Block = substreams::proto::decode_ptr(blk_ptr, blk_len)
            .expect("Unable to decode Block");

        Ok(BenchOutput {
            count: blk.transactions.len() as u64,
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
pub extern "C" fn bench_small_allocs_solana(blk_ptr: *mut u8, blk_len: usize) {
    substreams::register_panic_hook();
    let func = || -> Result<BenchOutput, Error> {
        let blk: Block = substreams::proto::decode_ptr(blk_ptr, blk_len)
            .expect("Unable to decode Block");

        let mut strings = Vec::with_capacity(1000);

        for trx in &blk.transactions {
            if let Some(transaction) = &trx.transaction {
                if let Some(first_sig) = transaction.signatures.first() {
                    strings.push(format!("sig:{}", bs58::encode(first_sig).into_string()));
                }
            }
            if let Some(meta) = &trx.meta {
                for log in &meta.log_messages {
                    strings.push(format!("log:{}", log));
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
pub extern "C" fn bench_large_allocs_solana(blk_ptr: *mut u8, blk_len: usize) {
    substreams::register_panic_hook();
    let func = || -> Result<BenchOutput, Error> {
        let blk: Block = substreams::proto::decode_ptr(blk_ptr, blk_len)
            .expect("Unable to decode Block");

        let mut data = Vec::new();

        for trx in &blk.transactions {
            if let Some(transaction) = &trx.transaction {
                for sig in &transaction.signatures {
                    data.extend_from_slice(sig);
                }
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
pub extern "C" fn bench_mixed_solana(blk_ptr: *mut u8, blk_len: usize) {
    substreams::register_panic_hook();
    let func = || -> Result<LargeOutput, Error> {
        let blk: Block = substreams::proto::decode_ptr(blk_ptr, blk_len)
            .expect("Unable to decode Block");

        let output = LargeOutput::default();

        // Just decode and count transactions for now
        let _count = blk.transactions.len();

        Ok(output)
    };
    substreams::skip_empty_output();
    let result = func();
    if result.is_err() {
        panic!("{:?}", result.unwrap_err());
    }
    substreams::output(result.expect("already checked"));
}
