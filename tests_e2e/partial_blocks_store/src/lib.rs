mod pb;
use crate::pb::sf::acme::r#type::v1 as acme;
use crate::pb::test::output as pbtest;

use substreams::store::{StoreAdd, StoreAddInt64, StoreGet, StoreGetInt64, StoreNew};

#[substreams::handlers::store]
pub fn store_tx_counter(blk: acme::Block, store: StoreAddInt64) {
    let block_number = blk.header.as_ref().map_or(0, |h| h.height);
    let tx_count = blk.transactions.len() as i64;

    // Store the transaction count for this block using block number as key
    let key = format!("block:{}", block_number);
    store.add(0, &key, tx_count);

    // Also maintain a total counter
    store.add(0, "total", tx_count);

    substreams::log::info!(
        "Block {}: Added {} transactions to store",
        block_number,
        tx_count
    );
}

#[substreams::handlers::map]
fn map_tx_counter_summary(
    blk: acme::Block,
    store: StoreGetInt64,
) -> Result<pbtest::TxCounterSummary, substreams::errors::Error> {
    let block_number = blk.header.as_ref().map_or(0, |h| h.height);
    let current_block_tx_count = blk.transactions.len() as i64;

    // Get the stored transaction count for this block
    let block_key = format!("block:{}", block_number);
    let stored_block_count = store.get_at(0, &block_key).unwrap_or(0);

    // Get total transaction count
    let total_tx_count = store.get_at(0, "total").unwrap_or(0);

    // Create a summary of recent block counts
    let mut block_counts = Vec::new();

    // Get counts for the last few blocks (for validation purposes)
    for i in 0..=5 {
        if block_number >= i {
            let check_block = block_number - i;
            let check_key = format!("block:{}", check_block);
            if let Some(count) = store.get_at(0, &check_key) {
                block_counts.push(pbtest::BlockTxCount {
                    block_number: check_block,
                    tx_count: count,
                });
            }
        }
    }

    let summary = pbtest::TxCounterSummary {
        block_number,
        current_block_tx_count,
        total_tx_count,
        block_counts,
    };

    substreams::log::info!(
        "Block {}: Current TX count: {}, Stored TX count: {}, Total: {}",
        block_number,
        current_block_tx_count,
        stored_block_count,
        total_tx_count
    );

    Ok(summary)
}
