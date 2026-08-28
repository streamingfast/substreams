mod pb;
use crate::pb::sf::acme::r#type::v1 as acme;
use hex_literal::hex;
use pb::test::clickhouse as pbclickhouse;
use pb::test::output as pbtest;
use substreams::Hex;

use num_traits::cast::ToPrimitive;
use std::str::FromStr;
use substreams::scalar::BigDecimal;
#[allow(unused_imports)]
use substreams::scalar::BigInt;
use substreams::store::{
    FoundationalStore, StoreAdd, StoreAddBigInt, StoreGet, StoreGetBigInt, StoreNew,
};
use substreams::skip_empty_output;

use prost::Message;
use prost_types::Any;
use substreams::pb::sf::substreams::foundational_store::model::v2::{
    Entry, Key, ResponseCode, SinkEntries,
};

// Type URL the hosted store tags our values with. It has to match the "Type URL"
// given when the store is created, and the one hosted-store.sh seeds with.
const STORE_VALUE_TYPE_URL: &str = "type.googleapis.com/test.output.StoreValue";

// Key rewritten on every block, so a consumer can tell a live store from a stalled one.
const LATEST_KEY: &str = "latest";

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

// ---------------------------------------------------------------------------
// Hosted Store modules
//
// map_hosted_store_feed populates a Substreams Feed Hosted Store; the two query
// modules read one back. The query modules live in substreams.hosted-store.yaml
// because their store identifier comes from the environment.
// ---------------------------------------------------------------------------

// Feeds a Substreams Feed Hosted Store. Two entries per block: `block:<height>`,
// which is written once and never changes, and `latest`, overwritten every block.
// if_not_exist stays false so `latest` can move.
#[substreams::handlers::map]
fn map_hosted_store_feed(blk: acme::Block) -> Result<SinkEntries, substreams::errors::Error> {
    let header = blk.header.as_ref();
    let value = pbtest::StoreValue {
        block_number: header.map_or(0, |h| h.height),
        block_hash: header.map_or(String::new(), |h| h.hash.clone()),
        transaction_count: blk.transactions.len() as u64,
    };

    let any = Any {
        type_url: STORE_VALUE_TYPE_URL.to_string(),
        value: value.encode_to_vec(),
    };

    Ok(SinkEntries {
        entries: vec![
            entry(format!("block:{}", value.block_number), any.clone()),
            entry(LATEST_KEY.to_string(), any),
        ],
        if_not_exist: false,
    })
}

// Reads back what map_hosted_store_feed wrote, at the block being processed. Against
// a store fed from the same chain, both keys report RESPONSE_CODE_FOUND on every block.
#[substreams::handlers::map]
fn map_query_substreams_feed_store(
    blk: acme::Block,
    store: FoundationalStore,
) -> Result<pbtest::StoreQuery, substreams::errors::Error> {
    let block_number = blk.header.as_ref().map_or(0, |h| h.height);

    Ok(query(
        block_number,
        &store,
        &[format!("block:{}", block_number), LATEST_KEY.to_string()],
    ))
}

// Reads the two fixed keys hosted-store.sh seeds over gRPC. Nothing on-chain writes
// them, so this is the module that proves the remote-feed path end to end: it hangs
// until the store is marked ready, then reports FOUND for both keys.
#[substreams::handlers::map]
fn map_query_remote_feed_store(
    blk: acme::Block,
    store: FoundationalStore,
) -> Result<pbtest::StoreQuery, substreams::errors::Error> {
    let block_number = blk.header.as_ref().map_or(0, |h| h.height);

    Ok(query(
        block_number,
        &store,
        &["dummy-key-1".to_string(), "dummy-key-2".to_string()],
    ))
}

fn entry(key: String, value: Any) -> Entry {
    Entry {
        key: Some(Key {
            bytes: key.into_bytes(),
        }),
        value: Some(value),
    }
}

// The store answers one QueriedEntry per requested key, in order, so results are
// matched back to their key positionally.
fn query(block_number: u64, store: &FoundationalStore, keys: &[String]) -> pbtest::StoreQuery {
    let response = store.get(keys);

    let entry = keys
        .iter()
        .enumerate()
        .map(|(index, key)| {
            let Some(queried) = response.entries.get(index) else {
                return pbtest::StoreQueryEntry {
                    key: key.clone(),
                    code: "NO_ENTRY".to_string(),
                    ..Default::default()
                };
            };

            let any = queried.entry.as_ref().and_then(|e| e.value.as_ref());

            pbtest::StoreQueryEntry {
                key: key.clone(),
                code: ResponseCode::try_from(queried.code)
                    .map(|code| code.as_str_name().to_string())
                    .unwrap_or_else(|_| format!("UNKNOWN({})", queried.code)),
                type_url: any.map_or(String::new(), |a| a.type_url.clone()),
                value: any.and_then(|a| pbtest::StoreValue::decode(a.value.as_slice()).ok()),
            }
        })
        .collect();

    pbtest::StoreQuery {
        block_number,
        entry,
    }
}

// Same events as map_events, emitted as test.clickhouse.Events so the message
// carries the ClickHouse table annotations. Output module for the
// substreams.clickhouse.yaml manifest.
#[substreams::handlers::map]
fn map_events_clickhouse(blk: acme::Block) -> Result<pbclickhouse::Events, substreams::errors::Error> {
    let mut events = pbclickhouse::Events::default();

    for transaction in blk.transactions {
        events.event.push(pbclickhouse::Event {
            evt_tx_hash: transaction.hash,
            evt_from: transaction.sender,
            evt_to: transaction.receiver,
            evt_block_number: blk.header.as_ref().map_or(0, |h| h.height),
            fee: transaction.fee.as_ref().map_or(0, |f| {
                f.bytes
                    .iter()
                    .take(8)
                    .enumerate()
                    .fold(0u64, |acc, (i, &b)| acc | ((b as u64) << (i * 8)))
            }),
        });
    }

    Ok(events)
}
