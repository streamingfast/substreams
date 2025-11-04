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
