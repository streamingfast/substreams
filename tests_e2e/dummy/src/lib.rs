mod pb;
use crate::pb::sf::acme::r#type::v1 as acme;
use hex_literal::hex;
use pb::test::output as pbtest;
use substreams::Hex;

use num_traits::cast::ToPrimitive;
use std::str::FromStr;
use substreams::scalar::BigDecimal;
#[allow(unused_imports)]
use substreams::scalar::BigInt;
use substreams::store::{StoreAdd, StoreAddBigInt, StoreGet, StoreGetBigInt, StoreNew};
use substreams::skip_empty_output;

#[substreams::handlers::map]
fn map_events(blk: acme::Block) -> Result<pbtest::Events, substreams::errors::Error> {
    let mut events = pbtest::Events::default();

    for transaction in blk.transactions {
        let event = pbtest::Event {
            evt_tx_hash: transaction.hash,
            evt_from: transaction.sender,
            evt_to: transaction.receiver,
            evt_block_number: blk.header.as_ref().map_or(0, |h| h.height),
            fee: transaction.fee.as_ref().map_or(0, |f| {
                // Convert BigInt bytes to u64 (assuming little-endian format)
                let bytes = &f.bytes;
                if bytes.is_empty() {
                    0
                } else {
                    let mut value = 0u64;
                    for (i, &byte) in bytes.iter().take(8).enumerate() {
                        value |= (byte as u64) << (i * 8);
                    }
                    value
                }
            }),
        };
        events.event.push(event);
    }

    Ok(events)
}

// identical to map_events but different start block
#[substreams::handlers::map]
fn map_events_0(blk: acme::Block) -> Result<pbtest::Events, substreams::errors::Error> {
    let mut events = pbtest::Events::default();

    for transaction in blk.transactions {
        let event = pbtest::Event {
            evt_tx_hash: transaction.hash,
            evt_from: transaction.sender,
            evt_to: transaction.receiver,
            evt_block_number: blk.header.as_ref().map_or(0, |h| h.height),
            fee: transaction.fee.as_ref().map_or(0, |f| {
                // Convert BigInt bytes to u64 (assuming little-endian format)
                let bytes = &f.bytes;
                if bytes.is_empty() {
                    0
                } else {
                    let mut value = 0u64;
                    for (i, &byte) in bytes.iter().take(8).enumerate() {
                        value |= (byte as u64) << (i * 8);
                    }
                    value
                }
            }),
        };
        events.event.push(event);
    }

    Ok(events)
}

#[substreams::handlers::map]
fn map_stats(
    blk: acme::Block,
    store: StoreGetBigInt,
) -> Result<pbtest::Events, substreams::errors::Error> {
    let mut events = pbtest::Events::default();

    for transaction in blk.transactions {
        let event = pbtest::Event {
            evt_tx_hash: transaction.hash,
            evt_from: transaction.sender,
            evt_to: transaction.receiver,
            evt_block_number: blk.header.as_ref().map_or(0, |h| h.height),
            fee: transaction.fee.as_ref().map_or(0, |f| {
                // Convert BigInt bytes to u64 (assuming little-endian format)
                let bytes = &f.bytes;
                if bytes.is_empty() {
                    0
                } else {
                    let mut value = 0u64;
                    for (i, &byte) in bytes.iter().take(8).enumerate() {
                        value |= (byte as u64) << (i * 8);
                    }
                    value
                }
            }),
        };
        events.event.push(event);
    }

    Ok(events)
}

#[substreams::handlers::store]
pub fn store_stats(blk: acme::Block, store: StoreAddBigInt) {
    store.add(0, "transactions", BigInt::from(blk.transactions.len()));
    store.add(
        0,
        blk.header.unwrap().hash,
        BigInt::from(blk.transactions.len()),
    );
}

#[substreams::handlers::store]
pub fn store_stats2(blk: acme::Block, store1: StoreGetBigInt, store2: StoreAddBigInt) {
    store2.add(0, "transactions", BigInt::from(blk.transactions.len()));
    store2.add(
        0,
        blk.header.unwrap().hash,
        BigInt::from(blk.transactions.len()),
    );
}

#[substreams::handlers::map]
fn map_stats2(
    blk: acme::Block,
    store: StoreGetBigInt,
) -> Result<pbtest::Events, substreams::errors::Error> {
    let mut events = pbtest::Events::default();

    for transaction in blk.transactions {
        let event = pbtest::Event {
            evt_tx_hash: transaction.hash,
            evt_from: transaction.sender,
            evt_to: transaction.receiver,
            evt_block_number: blk.header.as_ref().map_or(0, |h| h.height),
            fee: transaction.fee.as_ref().map_or(0, |f| {
                // Convert BigInt bytes to u64 (assuming little-endian format)
                let bytes = &f.bytes;
                if bytes.is_empty() {
                    0
                } else {
                    let mut value = 0u64;
                    for (i, &byte) in bytes.iter().take(8).enumerate() {
                        value |= (byte as u64) << (i * 8);
                    }
                    value
                }
            }),
        };
        events.event.push(event);
    }

    Ok(events)
}

// map_filtered takes map_events as its only input (no block source).
// It only keeps events from odd block numbers, calling skip_empty_output()
// when the result is empty. This causes the engine to skip caching the output,
// so that map_downstream sees nil inputs and hits the ErrNoInput path.
#[substreams::handlers::map]
fn map_filtered(events: pbtest::Events) -> Result<pbtest::Events, substreams::errors::Error> {
    let filtered: Vec<pbtest::Event> = events
        .event
        .into_iter()
        .filter(|e| e.evt_block_number % 2 == 1)
        .collect();

    if filtered.is_empty() {
        skip_empty_output();
        return Ok(pbtest::Events::default());
    }

    Ok(pbtest::Events { event: filtered })
}

// map_downstream takes only map_filtered as input (no direct block source).
// When map_filtered skips its output (even blocks), map_downstream has no input
// and will hit the ErrNoInput code path in the Go executor. The fix ensures
// that ErrNoInput still produces a properly-formed anypb.Any with TypeUrl set.
#[substreams::handlers::map]
fn map_downstream(events: pbtest::Events) -> Result<pbtest::Events, substreams::errors::Error> {
    Ok(events)
}
