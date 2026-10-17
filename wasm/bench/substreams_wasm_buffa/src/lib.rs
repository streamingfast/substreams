use substreams::errors::Error;
use substreams_ethereum::pb::eth::v2::{block::DetailLevel, Block};

/// Strips the extended-detail fields (call traces, system calls, balance/code
/// changes) from a block and marks it as `DETAILLEVEL_BASE`, mirroring what a
/// "light" (traces-stripped) block looks like. Receipt-level logs are left
/// untouched, so event decoding downstream is unaffected.
#[substreams::handlers::map]
fn map_light_block(mut block: Block) -> Result<Block, Error> {
    block.detail_level = DetailLevel::DETAILLEVEL_BASE.into();
    block.balance_changes.clear();
    block.code_changes.clear();
    block.system_calls.clear();
    for trx in &mut block.transaction_traces {
        trx.calls.clear();
    }

    Ok(block)
}

/// # Safety
///
/// `blk_ptr` must point to `blk_len` initialized bytes in this module's own linear
/// memory; the host (`wasm/wasmtime/heap.go`) guarantees this for every `SourceInput`
/// argument it writes before calling an entrypoint.
#[cfg(target_arch = "wasm32")]
#[no_mangle]
pub unsafe extern "C" fn map_count_logs_eager(blk_ptr: *mut u8, blk_len: usize) {
    use substreams_ethereum::buffa::Message;

    let bytes = std::slice::from_raw_parts(blk_ptr, blk_len);
    let block = Block::decode_from_slice(bytes).unwrap();
    let count = block.logs().count() as u64;
    substreams::output_raw(count.to_le_bytes().to_vec());
}

/// # Safety
///
/// Same contract as [`map_count_logs_eager`]: `blk_ptr`/`blk_len` must describe bytes
/// the host already wrote into this module's linear memory.
#[cfg(target_arch = "wasm32")]
#[no_mangle]
pub unsafe extern "C" fn map_count_logs_lazy(blk_ptr: *mut u8, blk_len: usize) {
    use substreams_ethereum::buffa::LazyMessageView;
    use substreams_ethereum::pb::eth::v2::BlockLazyView;

    let bytes = std::slice::from_raw_parts(blk_ptr, blk_len);
    let block = BlockLazyView::decode_lazy(bytes).unwrap();
    let count = block.logs().count() as u64;
    substreams::output_raw(count.to_le_bytes().to_vec());
}

#[cfg(test)]
mod tests {
    use super::*;
    use substreams::testing;
    use substreams_ethereum::buffa::Message;

    fn light_fixture(name: &str) {
        let path = format!("../testdata/{name}.binpb");
        let full = std::fs::read(&path).unwrap_or_else(|e| panic!("reading {path}: {e}"));

        let full_block = Block::decode_from_slice(full.as_slice()).unwrap();
        let full_log_count = full_block.logs().count();

        let light_block = testing::map!(map_light_block(full_block)).unwrap();
        let light_log_count = light_block.logs().count();
        assert_eq!(
            light_log_count, full_log_count,
            "stripping traces must not change the receipt log count"
        );

        let light_bytes = light_block.try_encode_to_bytes().unwrap();

        let out_path = format!("../testdata/{name}_light.binpb");
        std::fs::write(&out_path, &light_bytes).unwrap();

        println!(
            "{name}: full={} bytes, light={} bytes, logs={}",
            full.len(),
            light_bytes.len(),
            full_log_count
        );
    }

    #[test]
    fn generate_light_fixtures() {
        light_fixture("ethereum_mainnet_block_16021772");
        light_fixture("bnb_mainnet_block_78347457");
        light_fixture("polygon_mainnet_block_82340855");
    }

    /// (address, topics, data, ordinal) for one log, as plain owned bytes so eager and
    /// lazy decodes can be compared regardless of which borrows what.
    type LogFingerprint = (Vec<u8>, Vec<Vec<u8>>, Vec<u8>, u64);

    fn assert_lazy_matches_eager(bytes: &[u8]) {
        use substreams_ethereum::buffa::LazyMessageView;
        use substreams_ethereum::pb::eth::v2::BlockLazyView;

        let eager_logs: Vec<LogFingerprint> = Block::decode_from_slice(bytes)
            .unwrap()
            .logs()
            .map(|log| {
                (
                    log.address().to_vec(),
                    log.topics().iter().map(|t| t.to_vec()).collect(),
                    log.data().to_vec(),
                    log.ordinal(),
                )
            })
            .collect();

        let lazy_logs: Vec<LogFingerprint> = BlockLazyView::decode_lazy(bytes)
            .unwrap()
            .logs()
            .map(|log| {
                (
                    log.address.to_vec(),
                    log.topics.iter().map(|t| t.to_vec()).collect(),
                    log.data.to_vec(),
                    log.ordinal,
                )
            })
            .collect();

        assert!(
            !eager_logs.is_empty(),
            "fixture has zero logs, this check would pass vacuously"
        );
        assert_eq!(
            eager_logs, lazy_logs,
            "lazy view must decode the same (address, topics, data, ordinal) per log as eager decode, \
             not just the same count"
        );
    }

    /// Guards against the benchmark's lazy entrypoint being an accidental no-op: proves
    /// the lazy path reads the same real event data (not just the same count) as eager
    /// decode, on both full and light variants of every fixture.
    #[test]
    fn lazy_view_matches_eager_decode() {
        for name in [
            "ethereum_mainnet_block_16021772",
            "bnb_mainnet_block_78347457",
            "polygon_mainnet_block_82340855",
        ] {
            let full = std::fs::read(format!("../testdata/{name}.binpb")).unwrap();
            assert_lazy_matches_eager(&full);

            let full_block = Block::decode_from_slice(full.as_slice()).unwrap();
            let light_block = testing::map!(map_light_block(full_block)).unwrap();
            let light_bytes = light_block.try_encode_to_bytes().unwrap();
            assert_lazy_matches_eager(&light_bytes);
        }
    }
}
