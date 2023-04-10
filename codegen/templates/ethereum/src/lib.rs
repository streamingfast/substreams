mod abi;
mod pb;
use hex_literal::hex;
#[allow(unused_imports)]
use num_traits::ToPrimitive;
use pb::contrack;
use serde_json::json;
use substreams::Hex;
use substreams_ethereum::pb::eth::v2 as eth;
use substreams_ethereum::Event;

const CONTRACT: [u8; 20] = hex!("bc4ca0eda7647a8ab7c2061c2e118a18a936f13d");

substreams_ethereum::init!();

#[substreams::handlers::map]
fn map_events(blk: eth::Block) -> Result<contrack::Contrack, substreams::errors::Error> {
    Ok(contrack::Contrack {
        events: blk
            .receipts()
            .flat_map(|view| {
                view.receipt.logs.iter().filter_map(|log| {
                    if log.address != CONTRACT {
                        return None;
                    }
                    if let Some(event) = abi::contrack::events::Address::match_and_decode(log) {
                        return Some(
                            json!({
                                "block_number": blk.number,
                                "block_hash": Hex(&blk.hash).to_string(),
                                "trx_hash": Hex(&view.transaction.hash).to_string(),
                                "log_index": log.block_index,
                                "event_name": abi::contrack::events::Address::NAME,
                                "first": Hex(&event.first).to_string(),
                            })
                            .to_string()
                            .into_bytes(),
                        );
                    }
                    if let Some(event) = abi::contrack::events::ArrayOfAddress::match_and_decode(log) {
                        return Some(
                            json!({
                                "block_number": blk.number,
                                "block_hash": Hex(&blk.hash).to_string(),
                                "trx_hash": Hex(&view.transaction.hash).to_string(),
                                "log_index": log.block_index,
                                "event_name": abi::contrack::events::ArrayOfAddress::NAME,
                                "first": event.first.iter().map(|x| Hex(x).to_string()).collect::<Vec<_>>(),
                            })
                            .to_string()
                            .into_bytes(),
                        );
                    }
                    if let Some(event) = abi::contrack::events::Bytes::match_and_decode(log) {
                        return Some(
                            json!({
                                "block_number": blk.number,
                                "block_hash": Hex(&blk.hash).to_string(),
                                "trx_hash": Hex(&view.transaction.hash).to_string(),
                                "log_index": log.block_index,
                                "event_name": abi::contrack::events::Bytes::NAME,
                                "first": Hex(&event.first).to_string(),
                            })
                            .to_string()
                            .into_bytes(),
                        );
                    }
                    if let Some(event) = abi::contrack::events::FixedBytes::match_and_decode(log) {
                        return Some(
                            json!({
                                "block_number": blk.number,
                                "block_hash": Hex(&blk.hash).to_string(),
                                "trx_hash": Hex(&view.transaction.hash).to_string(),
                                "log_index": log.block_index,
                                "event_name": abi::contrack::events::FixedBytes::NAME,
                                "first": Hex(&event.first).to_string(),
                            })
                            .to_string()
                            .into_bytes(),
                        );
                    }
                    if let Some(event) = abi::contrack::events::Integer::match_and_decode(log) {
                        return Some(
                            json!({
                                "block_number": blk.number,
                                "block_hash": Hex(&blk.hash).to_string(),
                                "trx_hash": Hex(&view.transaction.hash).to_string(),
                                "log_index": log.block_index,
                                "event_name": abi::contrack::events::Integer::NAME,
                                "first": Into::<num_bigint::BigInt>::into(event.first).to_i64().unwrap(),
                            })
                            .to_string()
                            .into_bytes(),
                        );
                    }
                    if let Some(event) = abi::contrack::events::SignedFixedPoint::match_and_decode(log) {
                        return Some(
                            json!({
                                "block_number": blk.number,
                                "block_hash": Hex(&blk.hash).to_string(),
                                "trx_hash": Hex(&view.transaction.hash).to_string(),
                                "log_index": log.block_index,
                                "event_name": abi::contrack::events::SignedFixedPoint::NAME,
                                "first": event.first.to_string(),
                            })
                            .to_string()
                            .into_bytes(),
                        );
                    }
                    if let Some(event) = abi::contrack::events::Transfer::match_and_decode(log) {
                        return Some(
                            json!({
                                "block_number": blk.number,
                                "block_hash": Hex(&blk.hash).to_string(),
                                "trx_hash": Hex(&view.transaction.hash).to_string(),
                                "log_index": log.block_index,
                                "event_name": abi::contrack::events::Transfer::NAME,
                                "from": Hex(&event.from).to_string(),
                                "to": Hex(&event.to).to_string(),
                                "token_id": event.token_id.to_string(),
                            })
                            .to_string()
                            .into_bytes(),
                        );
                    }
                    if let Some(event) = abi::contrack::events::UnsignedInteger::match_and_decode(log) {
                        return Some(
                            json!({
                                "block_number": blk.number,
                                "block_hash": Hex(&blk.hash).to_string(),
                                "trx_hash": Hex(&view.transaction.hash).to_string(),
                                "log_index": log.block_index,
                                "event_name": abi::contrack::events::UnsignedInteger::NAME,
                                "first": event.first.to_string(),
                            })
                            .to_string()
                            .into_bytes(),
                        );
                    }

                    None
                })
            })
            .collect(),
    })
}
